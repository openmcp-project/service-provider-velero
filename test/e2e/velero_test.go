package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	klientresources "sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"

	velerosv1alpha1 "github.com/openmcp-project/service-provider-velero/api/v1alpha1"
	"github.com/openmcp-project/service-provider-velero/pkg/meta"

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
		Assess("verify service can be successfully consumed", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
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
		Assess("provider config update with new pull secret", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			return updateProviderConfigPullSecret(ctx, t, c)
		}).
		Assess("old pull secret is removed and new one is present on workload cluster", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			return verifyPullSecretRotation(ctx, t)
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
	mcp, err := clusterutils.MCPConfig(ctx, c, mcpName)
	mcp.WithNamespace("velero")
	if err != nil {
		t.Error(err)
		return ctx
	}
	// delete nginx deployment
	cl := kubernetes.NewForConfigOrDie(mcp.Client().RESTConfig())
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
	if err := wait.For(conditions.New(mcp.Client().Resources()).ResourceDeleted(ns)); err != nil {
		t.Error(err)
	}
	// restore from backup
	restore, err := resources.CreateObjectsFromDir(ctx, mcp.WithNamespace("velero"), "mcp/restore")
	if err != nil {
		t.Error(err)
		return ctx
	}
	// verify restore has been successful
	if err := wait.For(openmcpconditions.Status(&restore.Items[0], mcp, "phase", "Completed")); err != nil {
		t.Error(err)
	}
	// verify nginx deployment has been restored
	if err := wait.For(conditions.New(mcp.Client().Resources().WithNamespace("nginx-example")).
		DeploymentAvailable("nginx-deployment", "nginx-example")); err != nil {
		t.Error(err)
		return ctx
	}
	return ctx
}

func backup(ctx context.Context, t *testing.T, c *envconf.Config, mcpName, setupFolder string) context.Context {
	mcp, err := clusterutils.MCPConfig(ctx, c, mcpName)
	mcp.WithNamespace("velero")
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

func updateProviderConfigPullSecret(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
	if err := velerosv1alpha1.AddToScheme(c.Client().Resources().GetScheme()); err != nil {
		t.Errorf("failed to add api types to client scheme: %v", err)
		return ctx
	}
	providerConfig := &velerosv1alpha1.ProviderConfig{}
	providerConfig.SetName("velero")
	if err := c.Client().Resources().Get(ctx, "velero", "openmcp-system", providerConfig); err != nil {
		t.Errorf("failed to get provider config: %v", err)
		return ctx
	}
	providerConfig.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: "test-b"}}
	if err := c.Client().Resources().Update(ctx, providerConfig); err != nil {
		t.Errorf("failed to update provider config: %v", err)
	}
	// verify service stays healthy
	onboardingConfig, err := clusterutils.OnboardingConfig()
	velerosv1alpha1.AddToScheme(onboardingConfig.GetClient().Resources().GetScheme())
	if err != nil {
		t.Error(err)
		return ctx
	}
	velero := &velerosv1alpha1.Velero{}
	velero.SetName("test-aws-a")
	velero.SetNamespace(corev1.NamespaceDefault)
	if err := wait.For(openmcpconditions.Match(velero, onboardingConfig, "Ready", corev1.ConditionTrue), wait.WithTimeout(2*time.Minute)); err != nil {
		t.Errorf("Velero not ready after provider config update: %v", err)
	}
	return ctx
}

func verifyPullSecretRotation(ctx context.Context, t *testing.T) context.Context {
	workloadConfig, err := clusterutils.ConfigByPrefix("workload", "velero")
	if err != nil {
		t.Error(err)
		return ctx
	}
	res := workloadConfig.Client().Resources()
	labelSel := fmt.Sprintf("%s=%s", meta.LabelManagedBy, meta.LabelManagedByValue)
	// wait until no test-a secrets exist
	if err := wait.For(func(ctx context.Context) (bool, error) {
		list := &corev1.SecretList{}
		if err := res.List(ctx, list, klientresources.WithLabelSelector(labelSel)); err != nil {
			return false, err
		}
		for _, s := range list.Items {
			if s.Name == "test-a" {
				return false, nil
			}
		}
		return true, nil
	}); err != nil {
		t.Errorf("expected pull secret test-a to be deleted from workload cluster: %v", err)
		return ctx
	}
	// verify test-b secrets are present in all instance namespaces (one per MCP)
	if err := wait.For(func(ctx context.Context) (bool, error) {
		list := &corev1.SecretList{}
		if err := res.List(ctx, list, klientresources.WithLabelSelector(labelSel)); err != nil {
			return false, err
		}
		count := 0
		for _, s := range list.Items {
			if s.Name == "test-b" {
				count++
			}
		}
		return count == 2, nil
	}); err != nil {
		t.Errorf("expected 2 copies of pull secret test-b on workload cluster: %v", err)
	}
	return ctx
}
