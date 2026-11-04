package authn

import (
	"context"
	"fmt"

	"github.com/openmcp-project/extensibility-utils/pkg/tokenrefresh"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openmcp-project/service-provider-velero/pkg/resources"
)

const (
	serviceAccountMountPath = "/var/run/secrets/kubernetes.io/serviceaccount"
	serviceAccountVolume    = "kube-api-access"

	// TokenServiceProvider is the service provider name used to build the token annotation keys,
	// shared by the initial token write in Configure and the tokenrefresh controller.
	TokenServiceProvider = "velero"
)

// TokenApplyFunc injects the token to any container of the given PodSpec.
type TokenApplyFunc func(ps *corev1.PodSpec)

// ManagedServiceAccount references the managed ServiceAccount object
type ManagedServiceAccount struct {
	types.NamespacedName
}

func (m *ManagedServiceAccount) kubeAPIAccess() string {
	return fmt.Sprintf("kube-api-access-%s", m.Name)
}

// SecretRef returns a reference to the token secret created by this ManagedServiceAccount.
func (m *ManagedServiceAccount) SecretRef() corev1.LocalObjectReference {
	return corev1.LocalObjectReference{Name: m.kubeAPIAccess()}
}

// TokenBinding returns the ServiceAccount and the token Secret in the given workload namespace for the token refresh controller.
func (m *ManagedServiceAccount) TokenBinding(workloadNamespace string) tokenrefresh.Binding {
	return tokenrefresh.Binding{
		ServiceAccount: m.NamespacedName,
		Secret:         types.NamespacedName{Name: m.kubeAPIAccess(), Namespace: workloadNamespace},
	}
}

// Configure adds a managed ServiceAccount object to the given MCP cluster and a managed Secret object to the given workload cluster.
func (m *ManagedServiceAccount) Configure(workloadCluster, mcpCluster resources.ManagedCluster) TokenApplyFunc {
	// Add a service account on the remote cluster.
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name,
			Namespace: m.Namespace,
		},
	}
	msa := resources.NewManagedObject(sa, resources.ManagedObjectContext{
		ReconcileFunc: resources.NoOp,
		StatusFunc:    resources.SimpleStatus,
	})
	mcpCluster.AddObject(msa)

	// Add a secret on the local cluster that contains a token for the remote service account.
	secret := resources.NewManagedObject(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.kubeAPIAccess(),
			Namespace: workloadCluster.GetDefaultNamespace(),
		},
	}, resources.ManagedObjectContext{
		DependsOn: []resources.ManagedObject{
			msa,
		},
		// Mint the first token here so the Secret is never applied empty and crashloop until the tokenrefresh controller catches up.
		ReconcileFunc: func(ctx context.Context, o client.Object) error {
			tokenSecret := o.(*corev1.Secret)
			// if the token is already present, don't overwrite it, the tokenrefresh controller will handle it.
			if len(tokenSecret.Data[tokenrefresh.SecretKeyToken]) > 0 {
				return nil
			}
			_, err := tokenrefresh.EnsureToken(ctx, mcpCluster.GetCluster(), tokenSecret,
				m.TokenBinding(workloadCluster.GetDefaultNamespace()), TokenServiceProvider, tokenrefresh.DefaultTokenLifetime)
			return err
		},
		StatusFunc: resources.SimpleStatus,
	})
	workloadCluster.AddObject(secret)

	// Return a function that can be used to mount the service account token into a pod.
	return func(ps *corev1.PodSpec) {
		addOrReplaceVolume(ps, corev1.Volume{
			Name: serviceAccountVolume,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: m.kubeAPIAccess(),
					Optional:   ptr.To(false),
				},
			},
		})

		for i := range ps.Containers {
			applyToContainer(&ps.Containers[i], mcpCluster)
		}
		for i := range ps.InitContainers {
			applyToContainer(&ps.InitContainers[i], mcpCluster)
		}
	}
}

func addOrReplaceVolume(ps *corev1.PodSpec, vol corev1.Volume) {
	for i := range ps.Volumes {
		if ps.Volumes[i].Name == vol.Name {
			ps.Volumes[i] = vol
			return
		}
	}

	ps.Volumes = append(ps.Volumes, vol)
}

func addOrReplaceEnv(c *corev1.Container, env corev1.EnvVar) {
	for i := range c.Env {
		if c.Env[i].Name == env.Name {
			c.Env[i] = env
			return
		}
	}

	c.Env = append(c.Env, env)
}

func addOrReplaceVolumeMount(c *corev1.Container, vm corev1.VolumeMount) {
	for i := range c.VolumeMounts {
		if c.VolumeMounts[i].Name == vm.Name {
			c.VolumeMounts[i] = vm
			return
		}
	}

	c.VolumeMounts = append(c.VolumeMounts, vm)
}

func applyToContainer(c *corev1.Container, remoteCluster resources.ManagedCluster) {
	remoteHost, remotePort := remoteCluster.GetHostAndPort()

	addOrReplaceVolumeMount(c, corev1.VolumeMount{
		Name:      serviceAccountVolume,
		MountPath: serviceAccountMountPath,
		ReadOnly:  true,
	})
	addOrReplaceEnv(c, corev1.EnvVar{
		Name:  "KUBERNETES_SERVICE_HOST",
		Value: remoteHost,
	})
	addOrReplaceEnv(c, corev1.EnvVar{
		Name:  "KUBERNETES_SERVICE_PORT",
		Value: remotePort,
	})
}
