package secret

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/openmcp-project/controller-utils/pkg/clusters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openmcp-project/service-provider-velero/pkg/meta"
	"github.com/openmcp-project/service-provider-velero/pkg/resources"
	"github.com/openmcp-project/service-provider-velero/pkg/testutils"
)

func TestConfigure(t *testing.T) {
	const openmcpsystem = "openmcp-system"
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		workloadCluster  resources.ManagedCluster
		platformCluster  *clusters.Cluster
		imagePullSecrets []corev1.LocalObjectReference
		sourceNamespace  string
		// defined which managed objects are expected to result in an error
		wantErrors []string
	}{
		{
			name:             "no image pull secrets defined",
			workloadCluster:  resources.NewManagedCluster(testutils.CreateFakeCluster(t, "workload"), &rest.Config{}, "test", resources.WorkloadCluster),
			platformCluster:  testutils.CreateFakeCluster(t, "platform"),
			imagePullSecrets: nil,
			sourceNamespace:  openmcpsystem,
			wantErrors:       []string{},
		},
		{
			name:            "sync image pull secrets from platform to workload cluster",
			workloadCluster: resources.NewManagedCluster(testutils.CreateFakeCluster(t, "workload"), &rest.Config{}, "test", resources.WorkloadCluster),
			platformCluster: testutils.CreateFakeCluster(t, "platform", &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: openmcpsystem,
				},
				Data: map[string][]byte{
					"test": []byte("testdata"),
				},
				Type: corev1.SecretTypeDockerConfigJson,
			}),
			imagePullSecrets: []corev1.LocalObjectReference{
				{
					Name: "test",
				},
			},
			sourceNamespace: openmcpsystem,
			wantErrors:      []string{},
		},
		{
			name:            "requested to sync image pull secret that does not exist on platform cluster",
			workloadCluster: resources.NewManagedCluster(testutils.CreateFakeCluster(t, "workload"), &rest.Config{}, "test", resources.WorkloadCluster),
			platformCluster: testutils.CreateFakeCluster(t, "platform"),
			imagePullSecrets: []corev1.LocalObjectReference{
				{
					Name: "test",
				},
			},
			sourceNamespace: openmcpsystem,
			wantErrors:      []string{"test"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Configure(tt.workloadCluster, tt.platformCluster, tt.imagePullSecrets, tt.sourceNamespace)
			testutils.ExecApply(t, []resources.ManagedCluster{tt.workloadCluster}, len(tt.imagePullSecrets), tt.wantErrors)
			// verify any secret is synchronized between
			for _, ips := range tt.imagePullSecrets {
				sourceSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ips.Name,
						Namespace: tt.sourceNamespace,
					},
				}
				targetSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ips.Name,
						Namespace: tt.workloadCluster.GetDefaultNamespace(),
					},
				}

				if !slices.Contains(tt.wantErrors, ips.Name) {
					assert.NoError(t, tt.platformCluster.Client().Get(context.TODO(), client.ObjectKeyFromObject(sourceSecret), sourceSecret))
					assert.NoError(t, tt.workloadCluster.GetClient().Get(context.TODO(), client.ObjectKeyFromObject(targetSecret), targetSecret))
					assert.Equal(t, sourceSecret.Data, targetSecret.Data)
					assert.Equal(t, corev1.SecretTypeDockerConfigJson, targetSecret.Type, "secret type should be preserved")
				}
			}
		})
	}
}

func TestSecretCleaner_Cleanup(t *testing.T) {
	tests := []struct {
		name            string
		cluster         resources.ManagedCluster
		targetNamespace string
		secretsToKeep   []corev1.LocalObjectReference
		wantSecrets     []string
		wantResults     bool
		wantErr         bool
	}{
		{
			name:            "only managed secrets are deleted",
			targetNamespace: "velero",
			cluster: newFakeCluster(t, "velero", fake.NewClientBuilder().WithObjects(
				managedSecret("pull-secret", "velero"),
				unmanagedSecret("kube-api-access-velero-server", "velero"),
			).Build()),
			secretsToKeep: []corev1.LocalObjectReference{},
			wantSecrets:   []string{"kube-api-access-velero-server"},
		},
		{
			name:            "secrets in other namespaces are not deleted",
			targetNamespace: "other-ns",
			cluster: newFakeCluster(t, "other-ns", fake.NewClientBuilder().WithObjects(
				managedSecret("pull-secret", "velero"),
			).Build()),
			secretsToKeep: []corev1.LocalObjectReference{},
			wantSecrets:   []string{"pull-secret"},
		},
		{
			name:            "secrets to keep are not deleted",
			targetNamespace: "velero",
			cluster: newFakeCluster(t, "velero", fake.NewClientBuilder().WithObjects(
				managedSecret("pull-secret", "velero"),
				managedSecret("other-secret", "velero"),
			).Build()),
			secretsToKeep: []corev1.LocalObjectReference{
				{Name: "pull-secret"},
			},
			wantSecrets: []string{"pull-secret"},
		},
		{
			name:            "error is returned when list fails",
			targetNamespace: "velero",
			cluster:         newFakeCluster(t, "velero", listErrClient{}),
			secretsToKeep:   []corev1.LocalObjectReference{},
			wantErr:         true,
		},
		{
			name:            "result with error is returned when delete fails",
			targetNamespace: "velero",
			cluster: newFakeCluster(t, "velero", deleteErrClient{
				Client: fake.NewClientBuilder().WithObjects(
					managedSecret("pull-secret", "velero"),
				).Build(),
			}),
			secretsToKeep: []corev1.LocalObjectReference{},
			wantResults:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &secretCleaner{
				cluster:       tt.cluster,
				namespace:     tt.targetNamespace,
				secretsToKeep: tt.secretsToKeep,
			}

			results, err := c.Cleanup(context.Background())

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			if tt.wantResults {
				assert.NotEmpty(t, results)
				return
			}
			assert.Empty(t, results)

			secretList := &corev1.SecretList{}
			require.NoError(t, tt.cluster.GetClient().List(context.Background(), secretList))
			gotNames := make([]string, 0, len(secretList.Items))
			for _, s := range secretList.Items {
				gotNames = append(gotNames, s.Name)
			}
			assert.ElementsMatch(t, tt.wantSecrets, gotNames)
		})
	}
}

func managedSecret(name, namespace string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{meta.LabelManagedBy: meta.LabelManagedByValue},
		},
	}
}

func unmanagedSecret(name, namespace string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

func newFakeCluster(t *testing.T, namespace string, cl client.Client) resources.ManagedCluster {
	t.Helper()
	return resources.NewManagedCluster(testutils.CreateFakeClusterFromClient("workload", cl), &rest.Config{}, namespace, resources.WorkloadCluster)
}

type listErrClient struct{ client.Client }

func (l listErrClient) List(_ context.Context, _ client.ObjectList, _ ...client.ListOption) error {
	return errors.New("list failed")
}

type deleteErrClient struct{ client.Client }

func (d deleteErrClient) Delete(_ context.Context, _ client.Object, _ ...client.DeleteOption) error {
	return errors.New("delete failed")
}
