package authz

import (
	"context"
	"testing"

	"github.com/openmcp-project/controller-utils/pkg/clusters"
	"github.com/openmcp-project/service-provider-velero/pkg/authn"
	"github.com/openmcp-project/service-provider-velero/pkg/resources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestConfigure(t *testing.T) {
	const testNamespace = "test"
	tests := []struct {
		name string
		msa  *authn.ManagedServiceAccount
	}{
		{
			name: "create cluster role binding to cluster-admin role",
			msa: &authn.ManagedServiceAccount{
				NamespacedName: types.NamespacedName{
					Namespace: testNamespace,
					Name:      "msa",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			clientgoscheme.AddToScheme(scheme)

			// init workload cluster with workload objects
			mcpClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			cluster := resources.NewManagedCluster(clusters.NewTestClusterFromClient("client", mcpClient), &rest.Config{}, testNamespace, resources.WorkloadCluster)
			Configure(cluster, tt.msa)

			// invoke reconcile with manager
			mgr := resources.NewManager(testNamespace)
			mgr.AddCluster(cluster)
			result := mgr.Apply(context.TODO())

			// expected result contains a cluster role binding
			require.Len(t, result, 1)

			// verify cluster role binding
			require.Len(t, cluster.GetObjects(), 1)
			crb := &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name: "velero-server",
				},
			}
			assert.NoError(t, mcpClient.Get(context.TODO(), client.ObjectKeyFromObject(crb), crb))
			assert.Len(t, crb.Subjects, 1)
			assert.Equal(t, tt.msa.Name, crb.Subjects[0].Name)
			assert.Equal(t, tt.msa.Namespace, crb.Subjects[0].Namespace)
			assert.Equal(t, "cluster-admin", crb.RoleRef.Name)
		})
	}
}
