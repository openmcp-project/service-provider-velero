package authz

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openmcp-project/service-provider-velero/pkg/authn"
	"github.com/openmcp-project/service-provider-velero/pkg/resources"
	"github.com/openmcp-project/service-provider-velero/pkg/testutils"
)

func TestConfigure(t *testing.T) {
	const testNamespace = "test"
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		msa        *authn.ManagedServiceAccount
		cluster    resources.ManagedCluster
		wantErrors []string
	}{
		{
			name: "create cluster role binding to cluster-admin role",
			msa: &authn.ManagedServiceAccount{
				NamespacedName: types.NamespacedName{
					Namespace: testNamespace,
					Name:      "msa",
				},
			},
			cluster: resources.NewManagedCluster(testutils.CreateFakeCluster(t, "mcp"), &rest.Config{}, testNamespace, resources.WorkloadCluster),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Configure(tt.cluster, tt.msa)
			testutils.ExecApply(t, []resources.ManagedCluster{tt.cluster}, 1, tt.wantErrors)
			crb := &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name: "velero-server",
				},
			}
			assert.NoError(t, tt.cluster.GetClient().Get(context.TODO(), client.ObjectKeyFromObject(crb), crb))
			assert.Len(t, crb.Subjects, 1)
			assert.Equal(t, tt.msa.Name, crb.Subjects[0].Name)
			assert.Equal(t, tt.msa.Namespace, crb.Subjects[0].Namespace)
			assert.Equal(t, "cluster-admin", crb.RoleRef.Name)
		})
	}
}
