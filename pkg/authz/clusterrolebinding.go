package authz

import (
	"context"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openmcp-project/extensibility-utils/pkg/objectmanager"

	"github.com/openmcp-project/service-provider-velero/pkg/authn"
)

const clusterRoleBindingName = "velero-server"

// Configure adds a managed ClusterRoleBinding object to the given cluster.
// The passed in service account is granted the cluster-admin role.
func Configure(cluster objectmanager.Cluster, msa *authn.ManagedServiceAccount) {
	crb := objectmanager.NewObject(&rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterRoleBindingName,
		},
	}, objectmanager.ObjectConfig{
		ReconcileFunc: func(_ context.Context, o client.Object) error {
			oCRB := o.(*rbacv1.ClusterRoleBinding)
			oCRB.Subjects = []rbacv1.Subject{
				{
					Kind:      rbacv1.ServiceAccountKind,
					Name:      msa.Name,
					Namespace: msa.Namespace,
				},
			}
			oCRB.RoleRef = rbacv1.RoleRef{
				APIGroup: rbacv1.GroupName,
				Kind:     "ClusterRole",
				Name:     "cluster-admin",
			}
			return nil
		},
		StatusFunc: objectmanager.SimpleStatus,
	})
	cluster.AddObject(crb)
}
