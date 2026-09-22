package namespace

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openmcp-project/extensibility-utils/pkg/objectmanager"

	"github.com/openmcp-project/service-provider-velero/pkg/testutils"
)

func TestConfigure(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		namespaceName  string
		cluster        objectmanager.Cluster
		deletionPolicy objectmanager.DeletionPolicy
		wantErrors     []string
	}{
		{
			name:           "create and delete namespace with deletion policy delete",
			namespaceName:  "test-namespace",
			cluster:        objectmanager.NewCluster(testutils.CreateFakeCluster(t, "mcp"), "test-namespace", objectmanager.ManagedControlPlane),
			deletionPolicy: objectmanager.Delete,
		},
		{
			name:           "create and delete namespace with deletion policy orphan",
			namespaceName:  "test-namespace",
			cluster:        objectmanager.NewCluster(testutils.CreateFakeCluster(t, "mcp"), "test-namespace", objectmanager.ManagedControlPlane),
			deletionPolicy: objectmanager.Orphan,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Configure(tt.cluster, tt.deletionPolicy)
			testutils.ExecApply(t, []objectmanager.Cluster{tt.cluster}, 1, tt.wantErrors)

			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: tt.namespaceName,
				},
			}
			assert.NoError(t, tt.cluster.Client().Get(context.TODO(), client.ObjectKeyFromObject(ns), ns))
			assert.Equal(t, tt.namespaceName, ns.Name)

			// delete namespace
			testutils.ExecDelete(t, []objectmanager.Cluster{tt.cluster}, 1, tt.wantErrors)

			// verify namespace deleted
			err := tt.cluster.Client().Get(context.TODO(), client.ObjectKeyFromObject(ns), ns)
			if tt.deletionPolicy == objectmanager.Delete {
				assert.Error(t, err)
				assert.True(t, errors.IsNotFound(err))
			}
			if tt.deletionPolicy == objectmanager.Orphan {
				assert.NoError(t, err)
			}
		})
	}
}
