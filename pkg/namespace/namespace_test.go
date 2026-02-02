package namespace

import (
	"context"
	"testing"

	"github.com/openmcp-project/controller-utils/pkg/clusters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openmcp-project/service-provider-velero/pkg/resources"
)

func TestConfigure(t *testing.T) {
	tests := []struct {
		name           string
		namespaceName  string
		deletionPolicy resources.DeletionPolicy
	}{
		{
			name:           "create and delete namespace with deletion policy delete",
			namespaceName:  "test-namespace",
			deletionPolicy: resources.Delete,
		},
		{
			name:           "create and delete namespace with deletion policy orphan",
			namespaceName:  "test-namespace",
			deletionPolicy: resources.Orphan,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			clientgoscheme.AddToScheme(scheme)

			// init workload cluster with workload objects
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			cluster := resources.NewManagedCluster(clusters.NewTestClusterFromClient("client", fakeClient), &rest.Config{}, tt.namespaceName, resources.WorkloadCluster)

			Configure(cluster, tt.deletionPolicy)
			mgr := resources.NewManager(tt.namespaceName)
			mgr.AddCluster(cluster)

			// create namespace
			result := mgr.Apply(context.TODO())

			// expected result contains a namespace
			require.Len(t, result, 1)

			// verify namespace created
			require.Len(t, cluster.GetObjects(), 1)
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: tt.namespaceName,
				},
			}
			assert.NoError(t, fakeClient.Get(context.TODO(), client.ObjectKeyFromObject(ns), ns))
			assert.Equal(t, tt.namespaceName, ns.Name)

			// delete namespace
			result = mgr.Delete(context.TODO())

			// expected result contains a namespace
			require.Len(t, result, 1)

			// verify namespace deleted
			err := fakeClient.Get(context.TODO(), client.ObjectKeyFromObject(ns), ns)
			if tt.deletionPolicy == resources.Delete {
				assert.Error(t, err)
				assert.True(t, errors.IsNotFound(err))
			}
			if tt.deletionPolicy == resources.Orphan {
				assert.NoError(t, err)
			}
		})
	}
}
