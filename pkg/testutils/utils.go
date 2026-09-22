package testutils

import (
	"context"
	"testing"

	"github.com/openmcp-project/controller-utils/pkg/clusters"
	"github.com/openmcp-project/extensibility-utils/pkg/objectmanager"
	clustersv1alpha1 "github.com/openmcp-project/openmcp-operator/api/clusters/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openmcp-project/service-provider-velero/api/v1alpha1"
)

// CreateFakeCluster sets up a cluster with a fake client
func CreateFakeCluster(t *testing.T, id string, clusterObjects ...client.Object) *clusters.Cluster {
	t.Helper()
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = apiextv1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	_ = clustersv1alpha1.AddToScheme(scheme)

	// init cluster with objects
	fakeClient := fake.NewClientBuilder().WithObjects(clusterObjects...).WithScheme(scheme).Build()
	return clusters.NewTestClusterFromClient(id, fakeClient)
}

// CreateFakeClusterFromClient sets up a cluster with the given client
func CreateFakeClusterFromClient(id string, cl client.Client) *clusters.Cluster {
	return clusters.NewTestClusterFromClient(id, cl)
}

// ExecApply sets up a manager for the provided clusters and invokes reconciliation of all managed objects
func ExecApply(t *testing.T, clusters []objectmanager.Cluster, expectedManagedObjects int, wantErrors []string) []objectmanager.Result {
	t.Helper()
	// invoke apply with manager
	mgr := objectmanager.NewManager("instance-id")
	for _, cluster := range clusters {
		mgr.AddCluster(cluster)
	}
	results, err := mgr.Apply(context.TODO())
	require.Equal(t, len(wantErrors) == 0, err == nil)
	return assertResult(t, results.Results, expectedManagedObjects, wantErrors)
}

// ExecDelete sets up a manager for the provided clusters and invokes deletion of all managed objects
func ExecDelete(t *testing.T, clusters []objectmanager.Cluster, expectedManagedObjects int, wantErrors []string) []objectmanager.Result {
	t.Helper()
	// invoke delete with manager
	mgr := objectmanager.NewManager("instance-id")
	for _, cluster := range clusters {
		mgr.AddCluster(cluster)
	}
	results, err := mgr.Delete(context.TODO())
	require.NoError(t, err)
	return assertResult(t, results.Results, expectedManagedObjects, wantErrors)
}

func assertResult(t *testing.T, results []objectmanager.Result, expectedManagedObjects int, wantErrors []string) []objectmanager.Result {
	t.Helper()
	assert.Len(t, results, expectedManagedObjects, "expected %d managed object(s), got %d managed object(s)")
	errcount := 0
	for _, r := range results {
		if r.Error != nil {
			// assert that an error is expected
			assert.Contains(t, wantErrors, r.Object.ClientObject().GetName(), "unexpected reconcile error of managed object %s", r.Object.ClientObject().GetName())
			errcount++
		}
	}
	// assert that the overall number of errors is expected
	assert.Equal(t, len(wantErrors), errcount, "expected %d reconcile error(s), got %d reconcile error(s)", len(wantErrors), errcount)
	return results
}
