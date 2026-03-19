package crds

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/openmcp-project/service-provider-velero/pkg/resources"
	"github.com/openmcp-project/service-provider-velero/pkg/testutils"
)

func TestParse(t *testing.T) {
	crds, err := Parse()
	assert.NoError(t, err)
	assert.Len(t, crds, 13)
}

func TestConfigure(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		cluster    resources.ManagedCluster
		wantErr    bool
		wantErrors []string
	}{
		{
			name:       "create and delete crds",
			cluster:    resources.NewManagedCluster(testutils.CreateFakeCluster(t, "mcp"), &rest.Config{}, "default", resources.ManagedControlPlane),
			wantErr:    false,
			wantErrors: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotErr := Configure(tt.cluster)
			if tt.wantErr {
				require.Error(t, gotErr)
			}
			results := testutils.ExecApply(t, []resources.ManagedCluster{tt.cluster}, 13, tt.wantErrors)
			retrieveCRDs(t, tt.cluster.GetClient(), results, controllerutil.OperationResultCreated)
			results = testutils.ExecDelete(t, []resources.ManagedCluster{tt.cluster}, 13, tt.wantErrors)
			retrieveCRDs(t, tt.cluster.GetClient(), results, resources.OperationResultOrphaned)
		})
	}
}

func retrieveCRDs(t *testing.T, c client.Client, results []resources.Result, opResult controllerutil.OperationResult) {
	t.Helper()
	for _, r := range results {
		obj := &apiextv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name: r.Object.GetObject().GetName(),
			},
		}
		assert.NoError(t, c.Get(context.TODO(), client.ObjectKeyFromObject(obj), obj))
		assert.Equal(t, opResult, r.OperationResult)
	}
}
