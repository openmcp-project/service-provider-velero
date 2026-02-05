package runtime

import (
	"context"
	"fmt"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/stretchr/testify/assert"

	"github.com/openmcp-project/service-provider-velero/api/v1alpha1"
	"github.com/openmcp-project/service-provider-velero/pkg/testutils"
)

func TestPCReconciler_Reconcile(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		providerConfig ProviderConfig
		req            ctrl.Request
		want           ctrl.Result
		wantErr        bool
	}{
		{
			name: "test notify on standard provider config",
			providerConfig: &v1alpha1.ProviderConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
			},
			req: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name: "test",
				},
			},
			want:    ctrl.Result{},
			wantErr: false,
		},
		{
			name: "test notify on provider config marked for deletion",
			providerConfig: &v1alpha1.ProviderConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					DeletionTimestamp: &metav1.Time{
						Time: time.Now(),
					},
					Finalizers: []string{"pc-finalizer"},
				},
			},
			req: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name: "test",
				},
			},
			want:    ctrl.Result{},
			wantErr: false,
		},
		{
			name: "test notify on provider config not found",
			providerConfig: &v1alpha1.ProviderConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
			},
			req: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name: "notfound",
				},
			},
			want:    ctrl.Result{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewPCReconciler[*v1alpha1.ProviderConfig]("test", func() *v1alpha1.ProviderConfig {
				return &v1alpha1.ProviderConfig{}
			}).
				WithPlatformCluster(testutils.CreateFakeCluster(t, "platform", tt.providerConfig)).
				WithUpdateChannel(make(chan event.GenericEvent, 1))
			got, gotErr := r.Reconcile(context.Background(), tt.req)
			pcUpdate := <-r.providerUpdateChannel
			fmt.Println(pcUpdate)
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
