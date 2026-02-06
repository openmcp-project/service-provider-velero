package e2e

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"

	clustersv1alpha1 "github.com/openmcp-project/openmcp-operator/api/clusters/v1alpha1"
	"github.com/openmcp-project/openmcp-testing/pkg/clusterutils"
	openmcpconditions "github.com/openmcp-project/openmcp-testing/pkg/conditions"
	"github.com/openmcp-project/openmcp-testing/pkg/providers"
	"github.com/openmcp-project/openmcp-testing/pkg/resources"
)

// TestServiceProvider tests the service provider with two tenants (MCPs) using different versions of Velero
// and different version of velero-plugin-for-aws to backup and restore a nginx deployment
func TestServiceProvider(t *testing.T) {
	var onboardingList unstructured.UnstructuredList
	basicProviderTest := features.New("provider test").
		Setup(func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			if _, err := resources.CreateObjectsFromDir(ctx, c, "platform"); err != nil {
				t.Errorf("failed to create platform cluster objects: %v", err)
			}
			return ctx
		}).
		Setup(providers.CreateMCP("test-aws-a")).
		Setup(providers.CreateMCP("test-aws-b")).
		Assess("verify service can be successfylly consumed", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			onboardingConfig, err := clusterutils.OnboardingConfig()
			if err != nil {
				t.Error(err)
				return ctx
			}
			objList, err := resources.CreateObjectsFromDir(ctx, onboardingConfig, "onboarding")
			if err != nil {
				t.Errorf("failed to create onboarding cluster objects: %v", err)
				return ctx
			}
			for _, obj := range objList.Items {
				if err := wait.For(openmcpconditions.Match(&obj, onboardingConfig, "Ready", corev1.ConditionTrue)); err != nil {
					t.Error(err)
				}
			}
			objList.DeepCopyInto(&onboardingList)
			return ctx
		}).
		Assess("workload cluster fake backend", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			workloadConfig, err := clusterutils.ConfigByPrefix("workload", "velero")
			if err != nil {
				t.Error(err)
				return ctx
			}
			_, err = resources.CreateObjectsFromDir(ctx, workloadConfig, "workload")
			if err != nil {
				t.Error(err)
				return ctx
			}
			// wait for minio (s3 compatible object storage) to be available
			if err := wait.For(conditions.New(workloadConfig.Client().Resources()).
				DeploymentAvailable("minio", "velero")); err != nil {
				t.Error(err)
				return ctx
			}
			return ctx
		}).
		Assess("verify aws-a backup", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			return backup(ctx, t, c, "test-aws-a", "mcp/setup/aws-a")
		}).
		Assess("verify aws-b backup", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			return backup(ctx, t, c, "test-aws-b", "mcp/setup/aws-b")
		}).
		Assess("verify aws-a restore", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			return restore(ctx, t, c, "test-aws-a")
		}).
		Assess("verify aws-b restore", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			return restore(ctx, t, c, "test-aws-b")
		}).
		Teardown(func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			onboardingConfig, err := clusterutils.OnboardingConfig()
			if err != nil {
				t.Error(err)
				return ctx
			}
			for _, obj := range onboardingList.Items {
				if err := resources.DeleteObject(ctx, onboardingConfig, &obj, wait.WithTimeout(time.Minute)); err != nil {
					t.Errorf("failed to delete onboarding object: %v", err)
				}
			}
			return ctx
		}).
		Teardown(providers.DeleteMCP("test-aws-a", wait.WithTimeout(5*time.Minute))).
		Teardown(providers.DeleteMCP("test-aws-b", wait.WithTimeout(5*time.Minute)))
	testenv.Test(t, basicProviderTest.Feature())
}

func restore(ctx context.Context, t *testing.T, c *envconf.Config, mcpName string) context.Context {
	mcpPrefix := retrieveMCPClusterPrefix(ctx, t, c, mcpName)
	mcpConfig, err := clusterutils.ConfigByPrefix(mcpPrefix, "velero")
	if err != nil {
		t.Error(err)
		return ctx
	}
	// delete nginx deployment
	cl := kubernetes.NewForConfigOrDie(mcpConfig.Client().RESTConfig())
	ns, err := cl.CoreV1().Namespaces().Get(ctx, "nginx-example", metav1.GetOptions{})
	if err != nil {
		t.Error(err)
		return ctx
	}
	if err := cl.CoreV1().Namespaces().Delete(ctx, "nginx-example", metav1.DeleteOptions{}); err != nil {
		t.Error(err)
		return ctx
	}
	// verify nginx has been completely removed
	if err := wait.For(conditions.New(mcpConfig.Client().Resources()).ResourceDeleted(ns)); err != nil {
		t.Error(err)
	}
	// restore from backup
	restore, err := resources.CreateObjectsFromDir(ctx, mcpConfig.WithNamespace("velero"), "mcp/restore")
	if err != nil {
		t.Error(err)
		return ctx
	}
	// verify restore has been successful
	if err := wait.For(openmcpconditions.Status(&restore.Items[0], mcpConfig, "phase", "Completed")); err != nil {
		t.Error(err)
	}
	// verify nginx deployment has been restored
	if err := wait.For(conditions.New(mcpConfig.Client().Resources().WithNamespace("nginx-example")).
		DeploymentAvailable("nginx-deployment", "nginx-example")); err != nil {
		t.Error(err)
		return ctx
	}
	return ctx
}

func backup(ctx context.Context, t *testing.T, c *envconf.Config, mcpName, setupFolder string) context.Context {
	mcpPrefix := retrieveMCPClusterPrefix(ctx, t, c, mcpName)
	mcp, err := clusterutils.ConfigByPrefix(mcpPrefix, "velero")
	if err != nil {
		t.Error(err)
		return ctx
	}
	_, err = resources.CreateObjectsFromDir(ctx, mcp, setupFolder)
	if err != nil {
		t.Error(err)
		return ctx
	}

	// wait for velero to be ready for backups
	cl := dynamic.NewForConfigOrDie(mcp.Client().RESTConfig())
	backupStorageLocation, err := cl.Resource(schema.GroupVersionResource{
		Group:    "velero.io",
		Version:  "v1",
		Resource: "backupstoragelocations",
	}).Namespace("velero").Get(ctx, "default", metav1.GetOptions{})
	if err != nil {
		t.Error(err)
		return ctx
	}
	if err := wait.For(openmcpconditions.Status(backupStorageLocation, mcp, "phase", "Available")); err != nil {
		t.Error(err)
	}
	// create nginx example as backup/restore target
	_, err = resources.CreateObjectsFromDir(ctx, mcp.WithNamespace("nginx-example"), "mcp/setup/nginx")
	if err != nil {
		t.Error(err)
		return ctx
	}
	// wait for nginx deployment to be available
	if err := wait.For(conditions.New(mcp.Client().Resources().WithNamespace("nginx-example")).
		DeploymentAvailable("nginx-deployment", "nginx-example")); err != nil {
		t.Error(err)
		return ctx
	}
	// create backup
	backup, err := resources.CreateObjectsFromDir(ctx, mcp.WithNamespace("velero"), "mcp/backup")
	if err != nil {
		t.Error(err)
		return ctx
	}
	// verify backup has been successful
	if err := wait.For(openmcpconditions.Status(&backup.Items[0], mcp, "phase", "Completed")); err != nil {
		t.Error(err)
	}
	return ctx
}

func retrieveMCPClusterPrefix(ctx context.Context, t *testing.T, platformCluster *envconf.Config, mcpName string) string {
	cr := &clustersv1alpha1.ClusterRequest{}
	u := &unstructured.UnstructuredList{}
	u.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "clusters.openmcp.cloud",
		Version: "v1alpha1",
		Kind:    "ClusterRequest",
	})
	if err := platformCluster.Client().Resources().List(ctx, u); err != nil {
		t.Error(err)
		return ""
	}
	for _, item := range u.Items {
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.Object, cr); err != nil {
			t.Error(err)
			return ""
		}
		if cr.GetName() == mcpName {
			return cr.Status.Cluster.Name
		}
	}
	return ""
}
