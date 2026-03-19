package deployment

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openmcp-project/service-provider-velero/api/v1alpha1"
	"github.com/openmcp-project/service-provider-velero/pkg/authn"
	"github.com/openmcp-project/service-provider-velero/pkg/resources"
	"github.com/openmcp-project/service-provider-velero/pkg/testutils"
)

const testNamespace = "test"

func TestConfigure(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		cluster          resources.ManagedCluster
		namespace        string
		velero           *v1alpha1.Velero
		imagePullSecrets []corev1.LocalObjectReference
		images           map[string]string
		tokenApplyFunc   authn.TokenApplyFunc
		wantErrors       []string
		want             *appsv1.Deployment
	}{
		{
			name:    "create deployment with multiple plugins",
			cluster: resources.NewManagedCluster(testutils.CreateFakeCluster(t, "workload"), &rest.Config{}, testNamespace, resources.WorkloadCluster),
			velero: &v1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: testNamespace,
				},
				Spec: v1alpha1.VeleroSpec{
					Version: "v1",
					Plugins: []v1alpha1.VeleroPlugin{
						{
							Name:    "aws",
							Version: "v2",
						},
						{
							Name:    "gcp",
							Version: "v3",
						},
					},
				},
			},
			imagePullSecrets: []corev1.LocalObjectReference{
				{
					Name: "privateregcred",
				},
			},
			images: map[string]string{
				"velero": "v1",
				"aws":    "v2",
				"gcp":    "v3",
			},
			tokenApplyFunc: func(ps *corev1.PodSpec) {},
			wantErrors:     []string{},
			want:           &appsv1.Deployment{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Configure(tt.cluster, tt.namespace, tt.velero, tt.imagePullSecrets, tt.images, tt.tokenApplyFunc)
			testutils.ExecApply(t, []resources.ManagedCluster{tt.cluster}, 1, tt.wantErrors)
			// verify deployment exists
			dep := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "velero",
					Namespace: testNamespace,
				},
			}
			assert.NoError(t, tt.cluster.GetClient().Get(context.TODO(), client.ObjectKeyFromObject(dep), dep))

			// verify deployment
			assert.Equal(t, int32(1), *dep.Spec.Replicas)
			assert.Equal(t, dep.Spec.Template.Spec.ImagePullSecrets, tt.imagePullSecrets)

			// verify velero container image
			containers := dep.Spec.Template.Spec.Containers
			assert.Len(t, containers, 1)
			assert.Equal(t, containers[0].Name, "velero")
			assert.Equal(t, containers[0].Image, tt.images["velero"])

			// verify plugin images
			initContainers := dep.Spec.Template.Spec.InitContainers
			assert.Len(t, initContainers, len(tt.velero.Spec.Plugins))
			for _, p := range tt.velero.Spec.Plugins {
				assert.True(t, containsPlugin(initContainers, tt.images, p))
			}
		})
	}
}

func containsPlugin(initContainers []corev1.Container, images map[string]string, p v1alpha1.VeleroPlugin) bool {
	for _, c := range initContainers {
		if c.Image == images[p.Name] {
			return true
		}
	}
	return false
}

func TestConfigureMcp(t *testing.T) {
	tests := []struct {
		name       string // description of this test case
		cluster    resources.ManagedCluster
		image      string
		instance   string
		wantErrors []string
	}{
		{
			name:       "test scale zero mcp deployment",
			cluster:    resources.NewManagedCluster(testutils.CreateFakeCluster(t, "mcp"), &rest.Config{}, testNamespace, resources.ManagedControlPlane),
			image:      "velero/velero",
			instance:   "test",
			wantErrors: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ConfigureMcp(tt.cluster, tt.image, tt.instance)
			testutils.ExecApply(t, []resources.ManagedCluster{tt.cluster}, 1, tt.wantErrors)
			// verify deployment exists
			dep := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "velero",
					Namespace: testNamespace,
				},
			}
			assert.NoError(t, tt.cluster.GetClient().Get(context.TODO(), client.ObjectKeyFromObject(dep), dep))
			// verify deployment has 0 replicas
			assert.Equal(t, int32(0), *dep.Spec.Replicas)
		})
	}
}
