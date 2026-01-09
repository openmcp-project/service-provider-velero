package authn

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/openmcp-project/service-provider-velero/pkg/resources"
	"github.com/openmcp-project/service-provider-velero/pkg/schemes"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	serviceAccountMountPath = "/var/run/secrets/kubernetes.io/serviceaccount"
	serviceAccountVolume    = "kube-api-access"
)

var (
	ErrSANameOrNamespaceEmpty = errors.New("name or namespace in service account reference must not be empty")
	ErrRestConfigNil          = errors.New("rest config must not be nil")
	ErrExpirationInvalid      = errors.New("must not specify a duration less than 10 minutes")
)

type ServiceAccountToken struct {
	CAData      []byte
	Token       string
	TokenExpiry time.Time
}

type TokenApplyFunc func(ps *corev1.PodSpec)

// generateToken generates a token for the given ServiceAccount. If successful, it returns the token and the actual lifetime of it, which might deviate from the desired lifetime.
func generateToken(ctx context.Context, cfg *rest.Config, svcAccRef types.NamespacedName, expiration time.Duration) (*ServiceAccountToken, error) {
	if svcAccRef.Name == "" || svcAccRef.Namespace == "" {
		return nil, ErrSANameOrNamespaceEmpty
	}
	if cfg == nil {
		return nil, ErrRestConfigNil
	}
	if expiration < 10*time.Minute {
		return nil, ErrExpirationInvalid
	}

	client, err := client.New(cfg, client.Options{Scheme: schemes.Workload})
	if err != nil {
		return nil, err
	}

	sa := &corev1.ServiceAccount{}
	if err := client.Get(ctx, types.NamespacedName{Name: svcAccRef.Name, Namespace: svcAccRef.Namespace}, sa); err != nil {
		return nil, err
	}

	req := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			ExpirationSeconds: ptr.To(int64(expiration.Seconds())),
		},
	}
	if err := client.SubResource("token").Create(ctx, sa, req); err != nil {
		return nil, err
	}

	rc := &ServiceAccountToken{
		Token:       req.Status.Token,
		TokenExpiry: req.Status.ExpirationTimestamp.Time,
		CAData:      cfg.CAData,
	}

	if cfg.CAFile != "" {
		caBytes, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, err
		}
		rc.CAData = caBytes
	}

	return rc, nil
}

type ManagedServiceAccount struct {
	types.NamespacedName
}

func (m *ManagedServiceAccount) kubeAPIAccess() string {
	return fmt.Sprintf("kube-api-access-%s", m.Name)
}

// TODO optional image pull secret defined through providerconfig to enable pull from private registries
func (m *ManagedServiceAccount) Configure(localCluster, remoteCluster resources.ManagedCluster) TokenApplyFunc {
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
	remoteCluster.AddObject(msa)

	// Add a secret on the local cluster that contains a token for the remote service account.
	secret := resources.NewManagedObject(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.kubeAPIAccess(),
			Namespace: localCluster.GetDefaultNamespace(),
		},
	}, resources.ManagedObjectContext{
		DependsOn: []resources.ManagedObject{
			msa,
		},
		ReconcileFunc: func(ctx context.Context, o client.Object) error {
			oSecret := o.(*corev1.Secret)

			rc, err := generateToken(ctx, remoteCluster.GetConfig(), m.NamespacedName, 1*time.Hour)
			if err != nil {
				return err
			}

			oSecret.Data = map[string][]byte{
				"token":     []byte(rc.Token),
				"namespace": []byte(remoteCluster.GetDefaultNamespace()),
				"ca.crt":    []byte(rc.CAData),
			}

			return nil
		},
		StatusFunc: resources.SimpleStatus,
	})
	localCluster.AddObject(secret)

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
			applyToContainer(&ps.Containers[i], remoteCluster)
		}
		for i := range ps.InitContainers {
			applyToContainer(&ps.InitContainers[i], remoteCluster)
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
