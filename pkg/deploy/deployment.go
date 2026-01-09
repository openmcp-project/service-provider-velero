package deploy

import (
	"context"
	"fmt"

	"github.com/openmcp-project/service-provider-velero/api/v1alpha1"
	"github.com/openmcp-project/service-provider-velero/pkg/authn"
	"github.com/openmcp-project/service-provider-velero/pkg/resources"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func getPodLabels(instance *v1alpha1.Velero) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":     "velero",
		"app.kubernetes.io/instance": instance.Name,
	}
}

func Configure(localCluster resources.ManagedCluster, remoteNamespace string, velero *v1alpha1.Velero, tokenApplyFunc authn.TokenApplyFunc) {
	deployment := resources.NewManagedObject(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "velero",
			Namespace: localCluster.GetDefaultNamespace(),
		},
	}, resources.ManagedObjectContext{
		ReconcileFunc: func(ctx context.Context, o client.Object) error {
			oDeploy := o.(*appsv1.Deployment)

			labels := getPodLabels(velero)
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
						Containers: []corev1.Container{
							{
								Name:            "velero",
								Image:           fmt.Sprintf("%s:%s", velero.Spec.Image, velero.Spec.Version),
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
										Value: remoteNamespace,
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
					Image:           plugin.Image,
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
		StatusFunc: func(o client.Object) resources.Status {
			deploy := o.(*appsv1.Deployment)
			if !deploy.DeletionTimestamp.IsZero() {
				return resources.Status{
					Phase:   v1alpha1.Terminating,
					Message: "Deployment is terminating.",
				}
			}

			desired := ptr.Deref(deploy.Spec.Replicas, 1)
			ready := deploy.Status.ReadyReplicas

			if desired != ready {
				return resources.Status{
					Phase:   v1alpha1.Progressing,
					Message: "Waiting for all pods to become ready.",
				}
			}
			return resources.Status{
				Phase:   v1alpha1.Ready,
				Message: "All pods are ready.",
			}
		},
	})
	localCluster.AddObject(deployment)
}
