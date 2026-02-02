package namespace

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openmcp-project/service-provider-velero/pkg/resources"
	"github.com/openmcp-project/service-provider-velero/pkg/testutils"
)

func TestConfigure(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		namespaceName  string
		cluster        resources.ManagedCluster
		deletionPolicy resources.DeletionPolicy
		wantErrors     []string
	}{
		{
			name:           "create and delete namespace with deletion policy delete",
			namespaceName:  "test-namespace",
			cluster:        resources.NewManagedCluster(testutils.CreateFakeCluster(t, "mcp"), &rest.Config{}, "test-namespace", resources.ManagedControlPlane),
			deletionPolicy: resources.Delete,
		},
		{
			name:           "create and delete namespace with deletion policy orphan",
			namespaceName:  "test-namespace",
			cluster:        resources.NewManagedCluster(testutils.CreateFakeCluster(t, "mcp"), &rest.Config{}, "test-namespace", resources.ManagedControlPlane),
			deletionPolicy: resources.Orphan,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Configure(tt.cluster, tt.deletionPolicy)
			testutils.ExecApply(t, []resources.ManagedCluster{tt.cluster}, 1, tt.wantErrors)

			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: tt.namespaceName,
				},
			}
			assert.NoError(t, tt.cluster.GetClient().Get(context.TODO(), client.ObjectKeyFromObject(ns), ns))
			assert.Equal(t, tt.namespaceName, ns.Name)

			// delete namespace
			testutils.ExecDelete(t, []resources.ManagedCluster{tt.cluster}, 1, tt.wantErrors)

			// verify namespace deleted
			err := tt.cluster.GetClient().Get(context.TODO(), client.ObjectKeyFromObject(ns), ns)
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
