package runtime_test

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openmcp-project/controller-utils/pkg/clusters"
	clustersv1alpha1 "github.com/openmcp-project/openmcp-operator/api/clusters/v1alpha1"
	"github.com/openmcp-project/openmcp-operator/api/common"
	clusteraccess "github.com/openmcp-project/openmcp-operator/lib/clusteraccess"
	"github.com/stretchr/testify/assert"

	"github.com/openmcp-project/service-provider-velero/api/v1alpha1"
	spruntime "github.com/openmcp-project/service-provider-velero/pkg/runtime"
	"github.com/openmcp-project/service-provider-velero/pkg/testutils"
)

func TestSPReconciler_Reconcile(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		apiObj         spruntime.ServiceProviderAPI
		providerConfig *v1alpha1.ProviderConfig
		req            ctrl.Request
		want           ctrl.Result
		wantErr        bool
	}{
		{
			name: "api obj createOrUpdate -> requeue with pc poll interval",
			apiObj: &v1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
				},
			},
			req: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "test",
					Name:      "test",
				},
			},
			providerConfig: &v1alpha1.ProviderConfig{
				Spec: v1alpha1.ProviderConfigSpec{
					PollInterval: &metav1.Duration{
						Duration: time.Hour,
					},
				},
			},
			want: ctrl.Result{
				RequeueAfter: time.Hour,
			},
			wantErr: false,
		},
		{
			name: "api obj delete -> requeue with pc poll interval",
			apiObj: &v1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
					DeletionTimestamp: &metav1.Time{
						Time: time.Now(),
					},
					Finalizers: []string{"string"},
				},
			},
			req: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "test",
					Name:      "test",
				},
			},
			providerConfig: &v1alpha1.ProviderConfig{
				Spec: v1alpha1.ProviderConfigSpec{
					PollInterval: &metav1.Duration{
						Duration: time.Hour,
					},
				},
			},
			want: ctrl.Result{
				RequeueAfter: time.Hour,
			},
			wantErr: false,
		},
		{
			name: "api obj not found -> do not requeue",
			apiObj: &v1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
					DeletionTimestamp: &metav1.Time{
						Time: time.Now(),
					},
					Finalizers: []string{"string"},
				},
			},
			req: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "test",
					Name:      "notfound",
				},
			},
			providerConfig: &v1alpha1.ProviderConfig{
				Spec: v1alpha1.ProviderConfigSpec{
					PollInterval: &metav1.Duration{
						Duration: time.Hour,
					},
				},
			},
			want:    ctrl.Result{},
			wantErr: false,
		},
		{
			name: "provider config not found -> error",
			apiObj: &v1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
				},
			},
			req: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "test",
					Name:      "test",
				},
			},
			want:    ctrl.Result{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			onboardingCluster := testutils.CreateFakeCluster(t, "onboarding", tt.apiObj)
			platformCluster := testutils.CreateFakeCluster(t, "platform")

			r := spruntime.NewSPReconciler[*v1alpha1.Velero, *v1alpha1.ProviderConfig](func() *v1alpha1.Velero {
				return &v1alpha1.Velero{}
			}).
				WithOnboardingCluster(onboardingCluster).
				WithPlatformCluster(platformCluster).
				WithClusterAccessReconciler(FakeClusterAccessReconciler{
					ManagedControlPlane:   testutils.CreateFakeCluster(t, "mcp"),
					ManagedControlPlaneAR: &clustersv1alpha1.AccessRequest{},
					Workload:              testutils.CreateFakeCluster(t, "workload"),
					WorkloadAR:            &clustersv1alpha1.AccessRequest{},
				}).
				WithServiceProviderReconciler(fakeSPR).
				WithWorkloadCluster(true)
			if tt.providerConfig != nil {
				r.WithProviderConfig(tt.providerConfig)
			}
			got, gotErr := r.Reconcile(context.Background(), tt.req)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("Reconcile() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("Reconcile() succeeded unexpectedly")
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

var _ clusteraccess.Reconciler = FakeClusterAccessReconciler{}
var fakeSPR spruntime.ServiceProviderReconciler[*v1alpha1.Velero, *v1alpha1.ProviderConfig] = FakeServiceProviderReconciler{}

type FakeServiceProviderReconciler struct {
}

// CreateOrUpdate implements [runtime.ServiceProviderReconciler].
func (f FakeServiceProviderReconciler) CreateOrUpdate(_ context.Context, _ *v1alpha1.Velero, _ *v1alpha1.ProviderConfig, _ spruntime.ClusterContext) (ctrl.Result, error) {
	return reconcile.Result{}, nil
}

// Delete implements [runtime.ServiceProviderReconciler].
func (f FakeServiceProviderReconciler) Delete(_ context.Context, _ *v1alpha1.Velero, _ *v1alpha1.ProviderConfig, _ spruntime.ClusterContext) (ctrl.Result, error) {
	return reconcile.Result{}, nil
}

type FakeClusterAccessReconciler struct {
	ManagedControlPlane   *clusters.Cluster
	ManagedControlPlaneAR *clustersv1alpha1.AccessRequest
	Workload              *clusters.Cluster
	WorkloadAR            *clustersv1alpha1.AccessRequest
}

// MCPAccessRequest implements [clusteraccess.Reconciler].
func (f FakeClusterAccessReconciler) MCPAccessRequest(ctx context.Context, request reconcile.Request) (*clustersv1alpha1.AccessRequest, error) {
	return f.ManagedControlPlaneAR, nil
}

// MCPCluster implements [clusteraccess.Reconciler].
func (f FakeClusterAccessReconciler) MCPCluster(ctx context.Context, request reconcile.Request) (*clusters.Cluster, error) {
	return f.ManagedControlPlane, nil
}

// Reconcile implements [clusteraccess.Reconciler].
func (f FakeClusterAccessReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}

// ReconcileDelete implements [clusteraccess.Reconciler].
func (f FakeClusterAccessReconciler) ReconcileDelete(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}

// SkipWorkloadCluster implements [clusteraccess.Reconciler].
func (f FakeClusterAccessReconciler) SkipWorkloadCluster() clusteraccess.Reconciler {
	panic("unimplemented")
}

// WithMCPPermissions implements [clusteraccess.Reconciler].
func (f FakeClusterAccessReconciler) WithMCPPermissions(permissions []clustersv1alpha1.PermissionsRequest) clusteraccess.Reconciler {
	panic("unimplemented")
}

// WithMCPRoleRefs implements [clusteraccess.Reconciler].
func (f FakeClusterAccessReconciler) WithMCPRoleRefs(roleRefs []common.RoleRef) clusteraccess.Reconciler {
	panic("unimplemented")
}

// WithMCPScheme implements [clusteraccess.Reconciler].
func (f FakeClusterAccessReconciler) WithMCPScheme(scheme *runtime.Scheme) clusteraccess.Reconciler {
	panic("unimplemented")
}

// WithRetryInterval implements [clusteraccess.Reconciler].
func (f FakeClusterAccessReconciler) WithRetryInterval(interval time.Duration) clusteraccess.Reconciler {
	panic("unimplemented")
}

// WithWorkloadPermissions implements [clusteraccess.Reconciler].
func (f FakeClusterAccessReconciler) WithWorkloadPermissions(permissions []clustersv1alpha1.PermissionsRequest) clusteraccess.Reconciler {
	panic("unimplemented")
}

// WithWorkloadRoleRefs implements [clusteraccess.Reconciler].
func (f FakeClusterAccessReconciler) WithWorkloadRoleRefs(roleRefs []common.RoleRef) clusteraccess.Reconciler {
	panic("unimplemented")
}

// WithWorkloadScheme implements [clusteraccess.Reconciler].
func (f FakeClusterAccessReconciler) WithWorkloadScheme(scheme *runtime.Scheme) clusteraccess.Reconciler {
	panic("unimplemented")
}

// WorkloadAccessRequest implements [clusteraccess.Reconciler].
func (f FakeClusterAccessReconciler) WorkloadAccessRequest(ctx context.Context, request reconcile.Request) (*clustersv1alpha1.AccessRequest, error) {
	return f.WorkloadAR, nil
}

// WorkloadCluster implements [clusteraccess.Reconciler].
func (f FakeClusterAccessReconciler) WorkloadCluster(ctx context.Context, request reconcile.Request) (*clusters.Cluster, error) {
	return f.Workload, nil
}
