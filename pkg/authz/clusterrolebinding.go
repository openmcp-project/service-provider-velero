package authz

import (
	"context"

	"github.com/openmcp-project/service-provider-velero/pkg/authn"
	"github.com/openmcp-project/service-provider-velero/pkg/resources"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ClusterRoleBindingName = "velero-server"
)

func Configure(cluster resources.ManagedCluster, msa *authn.ManagedServiceAccount) {
	crb := resources.NewManagedObject(&rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: ClusterRoleBindingName,
		},
	}, resources.ManagedObjectContext{
		ReconcileFunc: func(ctx context.Context, o client.Object) error {
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
		StatusFunc: resources.SimpleStatus,
	})
	cluster.AddObject(crb)
}
