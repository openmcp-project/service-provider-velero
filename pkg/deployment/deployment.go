package deployment

import (
	"context"
	"fmt"
	"slices"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openmcp-project/service-provider-velero/api/v1alpha1"
	"github.com/openmcp-project/service-provider-velero/pkg/authn"
	"github.com/openmcp-project/service-provider-velero/pkg/instance"
	"github.com/openmcp-project/service-provider-velero/pkg/resources"
)

func getPodLabels(instance string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":     "velero",
		"app.kubernetes.io/instance": instance,
	}
}

// Configure add a managed Velero server deployment to the given cluster.
func Configure(cluster resources.ManagedCluster, namespace string, velero *v1alpha1.Velero, imagePullSecrets []corev1.LocalObjectReference, images map[string]string, tokenApplyFunc authn.TokenApplyFunc) {
	deployment := resources.NewManagedObject(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "velero",
			Namespace: cluster.GetDefaultNamespace(),
		},
	}, resources.ManagedObjectContext{
		ReconcileFunc: func(_ context.Context, o client.Object) error {
			oDeploy := o.(*appsv1.Deployment)

			labels := getPodLabels(instance.GetID(velero))
			for key, value := range labels {
				metav1.SetMetaDataLabel(&oDeploy.ObjectMeta, key, value)
			}

			oDeploy.Spec = appsv1.DeploymentSpec{
				Replicas: ptr.To[int32](1),
				Strategy: appsv1.DeploymentStrategy{
					Type: appsv1.RecreateDeploymentStrategyType,
				},
				Selector: &metav1.LabelSelector{
					MatchLabels: labels,
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: labels,
					},
					Spec: corev1.PodSpec{
						RestartPolicy:                 corev1.RestartPolicyAlways,
						TerminationGracePeriodSeconds: ptr.To[int64](10),
						ImagePullSecrets:              slices.Clone(imagePullSecrets),
						Containers: []corev1.Container{
							{
								Name:            "velero",
								Image:           images["velero"],
								ImagePullPolicy: corev1.PullIfNotPresent,
								Command:         []string{"/velero"},
								Args:            []string{"server"},
								Env: []corev1.EnvVar{
									{
										Name:  "VELERO_SCRATCH_DIR",
										Value: "/scratch",
									},
									{
										Name:  "VELERO_NAMESPACE",
										Value: namespace,
									},
									{
										Name:  "LD_LIBRARY_PATH",
										Value: "/plugins",
									},
								},
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      "plugins",
										MountPath: "/plugins",
									},
									{
										Name:      "scratch",
										MountPath: "/scratch",
									},
								},
							},
						},
						Volumes: []corev1.Volume{
							{
								Name: "plugins",
								VolumeSource: corev1.VolumeSource{
									EmptyDir: &corev1.EmptyDirVolumeSource{},
								},
							},
							{
								Name: "scratch",
								VolumeSource: corev1.VolumeSource{
									EmptyDir: &corev1.EmptyDirVolumeSource{},
								},
							},
						},
					},
				},
			}

			for i, plugin := range velero.Spec.Plugins {
				oDeploy.Spec.Template.Spec.InitContainers = append(oDeploy.Spec.Template.Spec.InitContainers, corev1.Container{
					Name:            fmt.Sprintf("plugin-%d", i),
					Image:           images[plugin.Name],
					ImagePullPolicy: corev1.PullIfNotPresent,
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "plugins",
							MountPath: "/target",
						},
					},
				})
			}

			tokenApplyFunc(&oDeploy.Spec.Template.Spec)
			return nil
		},
		StatusFunc: func(o client.Object, rl v1alpha1.ResourceLocation) resources.Status {
			deploy := o.(*appsv1.Deployment)
			if !deploy.DeletionTimestamp.IsZero() {
				return resources.Status{
					Phase:    v1alpha1.Terminating,
					Message:  "Deployment is terminating.",
					Location: rl,
				}
			}

			desired := ptr.Deref(deploy.Spec.Replicas, 1)
			ready := deploy.Status.ReadyReplicas

			if desired != ready {
				return resources.Status{
					Phase:    v1alpha1.Progressing,
					Message:  "Waiting for all pods to become ready.",
					Location: rl,
				}
			}
			return resources.Status{
				Phase:    v1alpha1.Ready,
				Message:  "All pods are ready.",
				Location: rl,
			}
		},
	})
	cluster.AddObject(deployment)
}

// ConfigureMcp adds a managed Velero deployment object to the given cluster.
func ConfigureMcp(cluster resources.ManagedCluster, image string, instance string) {
	// workaround for Velero expecting a deployment called 'velero' on the same cluster it watches its API/CRDs
	// we deploy a 0 scale deployment on the mcp
	deployment := resources.NewManagedObject(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "velero",
			Namespace: cluster.GetDefaultNamespace(),
		},
	}, resources.ManagedObjectContext{
		ReconcileFunc: func(_ context.Context, o client.Object) error {
			oDeploy := o.(*appsv1.Deployment)

			labels := getPodLabels(instance)
			for key, value := range labels {
				metav1.SetMetaDataLabel(&oDeploy.ObjectMeta, key, value)
			}

			oDeploy.Spec = appsv1.DeploymentSpec{
				Replicas: ptr.To[int32](0),
				Selector: &metav1.LabelSelector{
					MatchLabels: labels,
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: labels,
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:            "velero",
								Image:           image,
								ImagePullPolicy: corev1.PullIfNotPresent,
							},
						},
					},
				},
			}
			return nil
		},
		StatusFunc: func(o client.Object, rl v1alpha1.ResourceLocation) resources.Status {
			deploy := o.(*appsv1.Deployment)
			if !deploy.DeletionTimestamp.IsZero() {
				return resources.Status{
					Phase:    v1alpha1.Terminating,
					Message:  "Deployment is terminating.",
					Location: rl,
				}
			}
			desired := ptr.Deref(deploy.Spec.Replicas, 1)
			ready := deploy.Status.ReadyReplicas
			if desired != ready {
				return resources.Status{
					Phase:    v1alpha1.Progressing,
					Message:  "Waiting for all pods to become ready.",
					Location: rl,
				}
			}
			return resources.Status{
				Phase:    v1alpha1.Ready,
				Message:  "All pods are ready.",
				Location: rl,
			}
		},
	})
	cluster.AddObject(deployment)
}
