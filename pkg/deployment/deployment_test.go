package deployment

import (
	"context"
	"testing"

	"github.com/openmcp-project/controller-utils/pkg/clusters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openmcp-project/service-provider-velero/api/v1alpha1"
	"github.com/openmcp-project/service-provider-velero/pkg/authn"
	"github.com/openmcp-project/service-provider-velero/pkg/resources"
)

const testNamespace = "test"

func TestConfigure(t *testing.T) {
	tests := []struct {
		name             string
		workloadObjects  []client.Object
		velero           *v1alpha1.Velero
		imagePullSecrets []corev1.LocalObjectReference
		images           map[string]string
		tokenApplyFunc   authn.TokenApplyFunc
		want             *appsv1.Deployment
	}{
		{
			name:            "create deployment with multiple plugins",
			workloadObjects: []client.Object{},
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
			want:           &appsv1.Deployment{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			clientgoscheme.AddToScheme(scheme)

			// init workload cluster with workload objects
			workClient := fake.NewClientBuilder().WithObjects(tt.workloadObjects...).WithScheme(scheme).Build()
			workCluster := resources.NewManagedCluster(clusters.NewTestClusterFromClient("workload", workClient), &rest.Config{}, testNamespace, resources.WorkloadCluster)
			Configure(workCluster, testNamespace, tt.velero, tt.imagePullSecrets, tt.images, tt.tokenApplyFunc)

			// invoke reconcile with manager
			mgr := resources.NewManager(testNamespace)
			mgr.AddCluster(workCluster)
			result := mgr.Apply(context.TODO())
			// expected result contains a deployment on the managed control plane
			require.Len(t, result, 1)
			require.Len(t, workCluster.GetObjects(), 1)

			// verify deployment exists
			dep := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "velero",
					Namespace: testNamespace,
				},
			}
			assert.NoError(t, workClient.Get(context.TODO(), client.ObjectKeyFromObject(dep), dep))

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
		name     string // description of this test case
		image    string
		instance string
	}{
		{
			name:     "test scale zero mcp deployment",
			image:    "velero/velero",
			instance: "test",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			clientgoscheme.AddToScheme(scheme)

			// init mcp cluster
			mcpClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			mcpCluster := resources.NewManagedCluster(clusters.NewTestClusterFromClient("mcp", mcpClient), &rest.Config{}, testNamespace, resources.ManagedControlPlane)
			ConfigureMcp(mcpCluster, tt.image, tt.instance)

			// invoke reconcile with manager
			mgr := resources.NewManager(testNamespace)
			mgr.AddCluster(mcpCluster)
			result := mgr.Apply(context.TODO())

			// expected result contains a deployment on the managed control plane
			require.Len(t, result, 1)
			require.Len(t, mcpCluster.GetObjects(), 1)

			// verify deployment exists
			dep := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "velero",
					Namespace: testNamespace,
				},
			}
			assert.NoError(t, mcpClient.Get(context.TODO(), client.ObjectKeyFromObject(dep), dep))

			// verify deployment has 0 replicas
			assert.Equal(t, int32(0), *dep.Spec.Replicas)
		})
	}
}
