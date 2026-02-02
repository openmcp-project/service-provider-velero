package authn_test

import (
	"context"
	"testing"
	"time"

	"github.com/openmcp-project/controller-utils/pkg/clusters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openmcp-project/service-provider-velero/pkg/authn"
	"github.com/openmcp-project/service-provider-velero/pkg/resources"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	serviceAccountMountPath = "/var/run/secrets/kubernetes.io/serviceaccount"
	serviceAccountVolume    = "kube-api-access"
	testNamespace           = "test"
	secretName              = "kube-api-access-velero-server"
	expAnnotation           = "velero.services.openmcp.cloud/token-expiration-time"
)

func TestManagedServiceAccount_Configure(t *testing.T) {
	existingExpirationTime := time.Now().Add(time.Hour).Format(time.RFC3339)
	existingCreationTime := metav1.NewTime(time.Now().Add(-1 * time.Minute).Truncate(time.Second))
	tests := []struct {
		name                string
		workloadObjects     []client.Object
		mcpObjects          []client.Object
		pollInterval        time.Duration
		wantSecretCreation  bool
		wantTokenGeneration bool
	}{
		{
			name:                "secret is initially created with new token",
			workloadObjects:     []client.Object{},
			mcpObjects:          []client.Object{},
			pollInterval:        time.Minute * 15,
			wantSecretCreation:  true,
			wantTokenGeneration: true,
		},
		{
			name: "existing secret requires new token",
			workloadObjects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      secretName,
						Namespace: testNamespace,
						Annotations: map[string]string{
							expAnnotation: existingExpirationTime,
						},
						CreationTimestamp: existingCreationTime,
					},
				},
			},
			mcpObjects:          []client.Object{},
			pollInterval:        2 * time.Hour,
			wantSecretCreation:  false,
			wantTokenGeneration: true,
		},
		{
			name: "existing secret does not require new token",
			workloadObjects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      secretName,
						Namespace: testNamespace,
						Annotations: map[string]string{
							expAnnotation: existingExpirationTime,
						},
						CreationTimestamp: existingCreationTime,
					},
				},
			},
			mcpObjects:          []client.Object{},
			pollInterval:        time.Minute * 15,
			wantSecretCreation:  false,
			wantTokenGeneration: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &authn.ManagedServiceAccount{
				NamespacedName: types.NamespacedName{
					Name:      "velero-server",
					Namespace: testNamespace,
				},
			}
			scheme := runtime.NewScheme()
			clientgoscheme.AddToScheme(scheme)

			// init workload cluster with workload objects
			workClient := fake.NewClientBuilder().WithObjects(tt.workloadObjects...).WithScheme(scheme).Build()
			workCluster := resources.NewManagedCluster(clusters.NewTestClusterFromClient("workload", workClient), &rest.Config{}, testNamespace, resources.WorkloadCluster)

			// init mcp cluster with mcp objects
			mcpClient := fake.NewClientBuilder().WithObjects(tt.mcpObjects...).WithScheme(scheme).Build()
			mcpCluster := resources.NewManagedCluster(clusters.NewTestClusterFromClient("mcp", mcpClient), &rest.Config{}, testNamespace, resources.ManagedControlPlane)

			// call function under test
			tokenApplyFunc := m.Configure(workCluster, mcpCluster, tt.pollInterval)

			// invoke reconcile with manager
			mgr := resources.NewManager(testNamespace)
			mgr.AddCluster(mcpCluster)
			mgr.AddCluster(workCluster)
			result := mgr.Apply(context.TODO())

			// expected result contains a service account on the mcp and a secret on the workload cluster
			require.Len(t, result, 2)

			// verify service account exists
			require.Len(t, mcpCluster.GetObjects(), 1)
			sa := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "velero-server",
					Namespace: testNamespace,
				},
			}
			assert.NoError(t, mcpClient.Get(context.TODO(), client.ObjectKeyFromObject(sa), sa))

			// verify secret exists
			require.Len(t, workCluster.GetObjects(), 1)

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
			}
			require.NoError(t, workClient.Get(context.TODO(), client.ObjectKeyFromObject(secret), secret))

			// workload cluster secret must have an expiration date
			require.NotEmpty(t, secret.GetAnnotations()[expAnnotation])

			// assert secret creation
			gotSecretCreation := !(secret.GetCreationTimestamp().Time.Equal(existingCreationTime.Time))
			assert.Equal(t, tt.wantSecretCreation, gotSecretCreation)

			// assert the expected token generation behavior
			gotTokenGeneration := secret.GetAnnotations()[expAnnotation] != existingExpirationTime
			assert.Equal(t, tt.wantTokenGeneration, gotTokenGeneration)

			// apply the to dummy pod
			dummyPodSpec := &corev1.PodSpec{
				InitContainers: []corev1.Container{
					{
						Name: "init",
					},
				},
				Containers: []corev1.Container{
					{
						Name: "velero-server",
					},
				},
			}

			// test tokenApplyFunc behavior
			tokenApplyFunc(dummyPodSpec)

			// verify token mount to be used as in cluster config
			require.Len(t, dummyPodSpec.Volumes, 1)
			assert.Equal(t, serviceAccountVolume, dummyPodSpec.Volumes[0].Name)
			assert.Equal(t, secretName, dummyPodSpec.Volumes[0].Secret.SecretName)
			require.Len(t, dummyPodSpec.Containers, 1)
			require.Len(t, dummyPodSpec.InitContainers, 1)
			require.Len(t, dummyPodSpec.Containers[0].VolumeMounts, 1)
			assert.Equal(t, serviceAccountVolume, dummyPodSpec.Containers[0].VolumeMounts[0].Name)
			assert.Equal(t, serviceAccountMountPath, dummyPodSpec.Containers[0].VolumeMounts[0].MountPath)
			require.Len(t, dummyPodSpec.InitContainers[0].VolumeMounts, 1)
			assert.Equal(t, serviceAccountVolume, dummyPodSpec.InitContainers[0].VolumeMounts[0].Name)
			assert.Equal(t, serviceAccountMountPath, dummyPodSpec.InitContainers[0].VolumeMounts[0].MountPath)
		})
	}
}
