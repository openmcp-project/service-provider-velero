package secret

import (
	"context"
	"slices"
	"testing"

	"github.com/openmcp-project/controller-utils/pkg/clusters"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
				}
			}
		})
	}
}
