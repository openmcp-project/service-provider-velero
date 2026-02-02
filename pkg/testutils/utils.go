package testutils

import (
	"context"
	"testing"

	"github.com/openmcp-project/controller-utils/pkg/clusters"
	"github.com/stretchr/testify/assert"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openmcp-project/service-provider-velero/api/v1alpha1"
	"github.com/openmcp-project/service-provider-velero/pkg/resources"
)

// CreateFakeCluster sets up a cluster with a fake client
func CreateFakeCluster(t *testing.T, id string, clusterObjects ...client.Object) *clusters.Cluster {
	t.Helper()
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = apiextv1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	// init workload cluster with workload objects
	fakeClient := fake.NewClientBuilder().WithObjects(clusterObjects...).WithScheme(scheme).Build()
	return clusters.NewTestClusterFromClient(id, fakeClient)
}

// ExecApply sets up a manager for the provided clusters and invokes reconciliation of all managed objects
func ExecApply(t *testing.T, clusters []resources.ManagedCluster, expectedManagedObjects int, wantErrors []string) []resources.Result {
	t.Helper()
	// invoke reconcile with manager
	mgr := resources.NewManager("instance-id")
	for _, cluster := range clusters {
		mgr.AddCluster(cluster)
	}
	results := mgr.Apply(context.TODO())
	// expected result contains a deployment on the managed control plane
	assert.Len(t, results, expectedManagedObjects, "expected %d managed object(s), got %d managed object(s)")
	errcount := 0
	for _, r := range results {
		if r.Error != nil {
			// assert that an error is expected
			assert.Contains(t, wantErrors, r.Object.GetObject().GetName(), "unexpected reconcile error of managed object %s", r.Object.GetObject().GetName())
			errcount++
		}
	}
	// assert that the overall number of errors is expected
	assert.Equal(t, len(wantErrors), errcount, "expected %d reconcile error(s), got %d reconcile error(s)", len(wantErrors), errcount)
	return results
}

// ExecDelete sets up a manager for the provided clusters and invokes deletion of all managed objects
func ExecDelete(t *testing.T, clusters []resources.ManagedCluster, expectedManagedObjects int, wantErrors []string) []resources.Result {
	t.Helper()
	// invoke reconcile with manager
	mgr := resources.NewManager("instance-id")
	for _, cluster := range clusters {
		mgr.AddCluster(cluster)
	}
	results := mgr.Delete(context.TODO())
	// expected result contains a deployment on the managed control plane
	assert.Len(t, results, expectedManagedObjects, "expected %d managed object(s), got %d managed object(s)")
	errcount := 0
	for _, r := range results {
		if r.Error != nil {
			// assert that an error is expected
			assert.Contains(t, wantErrors, r.Object.GetObject().GetName(), "unexpected reconcile error of managed object %s", r.Object.GetObject().GetName())
			errcount++
		}
	}
	// assert that the overall number of errors is expected
	assert.Equal(t, len(wantErrors), errcount, "expected %d reconcile error(s), got %d reconcile error(s)", len(wantErrors), errcount)
	return results
}
