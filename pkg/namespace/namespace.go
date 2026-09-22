package namespace

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openmcp-project/extensibility-utils/pkg/objectmanager"
)

// Configure adds a managed Namespace object to the given ManagedCluster.
func Configure(cluster objectmanager.Cluster, deletionPolicy objectmanager.DeletionPolicy) {
	ns := objectmanager.NewObject(&corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: cluster.DefaultNamespace(),
		},
	}, objectmanager.ObjectConfig{
		ReconcileFunc:  objectmanager.NoOp,
		DeletionPolicy: deletionPolicy,
		StatusFunc:     objectmanager.SimpleStatus,
	})
	cluster.AddObject(ns)
}
