package e2e

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"

	"github.com/openmcp-project/openmcp-testing/pkg/clusterutils"
	openmcpconditions "github.com/openmcp-project/openmcp-testing/pkg/conditions"
	"github.com/openmcp-project/openmcp-testing/pkg/providers"
	"github.com/openmcp-project/openmcp-testing/pkg/resources"
)

func TestServiceProvider(t *testing.T) {
	var onboardingList unstructured.UnstructuredList
	basicProviderTest := features.New("provider test").
		Setup(func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			if _, err := resources.CreateObjectsFromDir(ctx, c, "platform"); err != nil {
				t.Errorf("failed to create platform cluster objects: %v", err)
			}
			return ctx
		}).
		Setup(providers.CreateMCP("test-mcp")).
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
		Assess("verify velero backup functionality", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			mcpConfig, err := clusterutils.ConfigByPrefix("mcp", "velero")
			if err != nil {
				t.Error(err)
				return ctx
			}
			_, err = resources.CreateObjectsFromDir(ctx, mcpConfig, "mcp/setup/velero")
			if err != nil {
				t.Error(err)
				return ctx
			}
			// wait for minio (s3 compatible object storage) to be available
			if err := wait.For(conditions.New(mcpConfig.Client().Resources()).
				DeploymentAvailable("minio", "velero")); err != nil {
				t.Error(err)
				return ctx
			}
			// wait for velero to be ready for backups
			cl := dynamic.NewForConfigOrDie(mcpConfig.Client().RESTConfig())
			backupStorageLocation, err := cl.Resource(schema.GroupVersionResource{
				Group:    "velero.io",
				Version:  "v1",
				Resource: "backupstoragelocations",
			}).Namespace("velero").Get(ctx, "default", v1.GetOptions{})
			if err != nil {
				t.Error(err)
				return ctx
			}
			if err := wait.For(openmcpconditions.Status(backupStorageLocation, mcpConfig, "phase", "Available")); err != nil {
				t.Error(err)
			}
			// create nginx example as backup/restore target
			_, err = resources.CreateObjectsFromDir(ctx, mcpConfig.WithNamespace("nginx-example"), "mcp/setup/nginx")
			if err != nil {
				t.Error(err)
				return ctx
			}
			// wait for nginx deployment to be available
			if err := wait.For(conditions.New(mcpConfig.Client().Resources().WithNamespace("nginx-example")).
				DeploymentAvailable("nginx-deployment", "nginx-example")); err != nil {
				t.Error(err)
				return ctx
			}
			// create backup
			backup, err := resources.CreateObjectsFromDir(ctx, mcpConfig.WithNamespace("velero"), "mcp/backup")
			if err != nil {
				t.Error(err)
				return ctx
			}
			// verify backup has been successful
			if err := wait.For(openmcpconditions.Status(&backup.Items[0], mcpConfig, "phase", "Completed")); err != nil {
				t.Error(err)
			}
			return ctx
		}).
		Assess("verify velero restore functionality", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			mcpConfig, err := clusterutils.ConfigByPrefix("mcp", "velero")
			if err != nil {
				t.Error(err)
				return ctx
			}
			// delete nginx deployment
			cl := kubernetes.NewForConfigOrDie(mcpConfig.Client().RESTConfig())
			ns, err := cl.CoreV1().Namespaces().Get(ctx, "nginx-example", v1.GetOptions{})
			if err != nil {
				t.Error(err)
				return ctx
			}
			if err := cl.CoreV1().Namespaces().Delete(ctx, "nginx-example", v1.DeleteOptions{}); err != nil {
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
		Teardown(providers.DeleteMCP("test-mcp", wait.WithTimeout(5*time.Minute)))
	testenv.Test(t, basicProviderTest.Feature())
}
