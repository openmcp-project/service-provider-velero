package authn_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openmcp-project/extensibility-utils/pkg/objectmanager"

	"github.com/openmcp-project/service-provider-velero/pkg/authn"
	"github.com/openmcp-project/service-provider-velero/pkg/testutils"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
		name string // description of this test case
		// Named input parameters for target function.
		workloadCluster     objectmanager.Cluster
		mcpCluster          objectmanager.Cluster
		pollInterval        time.Duration
		wantSecretCreation  bool
		wantTokenGeneration bool
		wantErrors          []string
	}{
		{
			name:                "secret is initially created with new token",
			workloadCluster:     objectmanager.NewCluster(testutils.CreateFakeCluster(t, "workload"), testNamespace, objectmanager.WorkloadCluster),
			mcpCluster:          objectmanager.NewCluster(testutils.CreateFakeCluster(t, "mcp").WithRESTConfig(&rest.Config{}), testNamespace, objectmanager.ManagedControlPlane),
			pollInterval:        time.Minute * 15,
			wantSecretCreation:  true,
			wantTokenGeneration: true,
		},
		{
			name: "existing secret requires new token",
			workloadCluster: objectmanager.NewCluster(testutils.CreateFakeCluster(t, "workload", &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						expAnnotation: existingExpirationTime,
					},
					CreationTimestamp: existingCreationTime,
				},
			},
			), testNamespace, objectmanager.WorkloadCluster),
			mcpCluster:          objectmanager.NewCluster(testutils.CreateFakeCluster(t, "mcp").WithRESTConfig(&rest.Config{}), testNamespace, objectmanager.ManagedControlPlane),
			pollInterval:        2 * time.Hour,
			wantSecretCreation:  false,
			wantTokenGeneration: true,
		},
		{
			name: "existing secret does not require new token",
			workloadCluster: objectmanager.NewCluster(testutils.CreateFakeCluster(t, "workload", &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						expAnnotation: existingExpirationTime,
					},
					CreationTimestamp: existingCreationTime,
				},
			},
			), testNamespace, objectmanager.WorkloadCluster),
			mcpCluster:          objectmanager.NewCluster(testutils.CreateFakeCluster(t, "mcp").WithRESTConfig(&rest.Config{}), testNamespace, objectmanager.ManagedControlPlane),
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
			// call function under test
			tokenApplyFunc := m.Configure(tt.workloadCluster, tt.mcpCluster, tt.pollInterval)
			testutils.ExecApply(t, []objectmanager.Cluster{tt.mcpCluster, tt.workloadCluster}, 2, tt.wantErrors)

			// verify service account exists
			require.Len(t, tt.mcpCluster.Objects(), 1)
			sa := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "velero-server",
					Namespace: testNamespace,
				},
			}
			assert.NoError(t, tt.mcpCluster.Client().Get(context.TODO(), client.ObjectKeyFromObject(sa), sa))

			// verify secret exists
			require.Len(t, tt.workloadCluster.Objects(), 1)

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
			}
			require.NoError(t, tt.workloadCluster.Client().Get(context.TODO(), client.ObjectKeyFromObject(secret), secret))

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
