/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/stretchr/testify/assert"

	apiv1alpha1 "github.com/openmcp-project/service-provider-velero/api/v1alpha1"
	"github.com/openmcp-project/service-provider-velero/pkg/resources"
	"github.com/openmcp-project/service-provider-velero/pkg/spruntime"
	"github.com/openmcp-project/service-provider-velero/pkg/testutils"
)

func TestVeleroReconciler_CreateOrUpdate(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		obj             *apiv1alpha1.Velero
		pc              *apiv1alpha1.ProviderConfig
		clusters        spruntime.ClusterContext
		manager         resources.Manager
		want            ctrl.Result
		wantErr         bool
		wantStatusPhase string
	}{
		{
			name: "managed objects ready -> status ready",
			obj: createVeleroObj("v1", createPlugins(map[string]string{
				"aws": "v2",
			})),
			pc: &apiv1alpha1.ProviderConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
				Spec: apiv1alpha1.ProviderConfigSpec{
					PollInterval: &metav1.Duration{
						Duration: time.Minute,
					},
					ImagePullSecrets: []corev1.LocalObjectReference{},
					AvailableImages: []apiv1alpha1.AvailableVeleroImages{
						{
							Name:     "velero",
							Versions: []string{"v1"},
							Image:    "velero/velero",
						},
						{
							Name:     "aws",
							Versions: []string{"v2"},
							Image:    "velero/aws",
						},
					},
				},
			},
			clusters: fakeClusterContext(t),
			manager: fakeManager{
				results: []resources.Result{
					fakeResult(apiv1alpha1.Ready, controllerutil.OperationResultCreated, resources.ManagedControlPlane, nil),
				},
			},
			want:            ctrl.Result{},
			wantStatusPhase: spruntime.StatusPhaseReady,
			wantErr:         false,
		},
		{
			name: "managed objects not ready -> status progressing",
			obj: createVeleroObj("v1", createPlugins(map[string]string{
				"aws": "v2",
			})),
			pc: &apiv1alpha1.ProviderConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
				Spec: apiv1alpha1.ProviderConfigSpec{
					PollInterval: &metav1.Duration{
						Duration: time.Minute,
					},
					ImagePullSecrets: []corev1.LocalObjectReference{},
					AvailableImages: []apiv1alpha1.AvailableVeleroImages{
						{
							Name:     "velero",
							Versions: []string{"v1"},
							Image:    "velero/velero",
						},
						{
							Name:     "aws",
							Versions: []string{"v2"},
							Image:    "velero/aws",
						},
					},
				},
			},
			clusters: fakeClusterContext(t),
			manager: fakeManager{
				results: []resources.Result{
					fakeResult(apiv1alpha1.Progressing, controllerutil.OperationResultCreated, resources.ManagedControlPlane, nil),
				},
			},
			want:            ctrl.Result{},
			wantStatusPhase: spruntime.StatusPhaseProgressing,
			wantErr:         false,
		},
		{
			name: "managed objects with errors -> error",
			obj: createVeleroObj("v1", createPlugins(map[string]string{
				"aws": "v2",
			})),
			pc: &apiv1alpha1.ProviderConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
				Spec: apiv1alpha1.ProviderConfigSpec{
					PollInterval: &metav1.Duration{
						Duration: time.Minute,
					},
					ImagePullSecrets: []corev1.LocalObjectReference{},
					AvailableImages: []apiv1alpha1.AvailableVeleroImages{
						{
							Name:     "velero",
							Versions: []string{"v1"},
							Image:    "velero/velero",
						},
						{
							Name:     "aws",
							Versions: []string{"v2"},
							Image:    "velero/aws",
						},
					},
				},
			},
			clusters: fakeClusterContext(t),
			manager: fakeManager{
				results: []resources.Result{
					fakeResult(apiv1alpha1.Failed, controllerutil.OperationResultCreated, resources.ManagedControlPlane, errors.New("test")),
				},
			},
			want:    ctrl.Result{},
			wantErr: true,
		},
		{
			name: "Velero version not available -> error",
			obj: createVeleroObj("v3", createPlugins(
				map[string]string{
					"aws": "v2",
				},
			)),
			pc: createProviderConfig([]apiv1alpha1.AvailableVeleroImages{
				{
					Name:     "velero",
					Versions: []string{"v1", "v2"},
					Image:    "velero/velero",
				},
				{
					Name:     "aws",
					Versions: []string{"v1", "v2"},
					Image:    "velero/aws",
				},
			}),
			clusters: fakeClusterContext(t),
			want:     ctrl.Result{},
			wantErr:  true,
		},
		{
			name: "Plugin version not available -> error",
			obj: createVeleroObj("v1", createPlugins(
				map[string]string{
					"aws": "v3",
				},
			)),
			pc: createProviderConfig([]apiv1alpha1.AvailableVeleroImages{
				{
					Name:     "velero",
					Versions: []string{"v1", "v2"},
					Image:    "velero/velero",
				},
				{
					Name:     "aws",
					Versions: []string{"v1", "v2"},
					Image:    "velero/aws",
				},
			}),
			clusters: fakeClusterContext(t),
			want:     ctrl.Result{},
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			onboardingCluster := testutils.CreateFakeCluster(t, "onboarding", tt.obj)
			r := &VeleroReconciler{
				OnboardingCluster: onboardingCluster,
				PlatformCluster:   testutils.CreateFakeCluster(t, "platform", tt.pc),
				PodNamespace:      "openmcp-system",
				CreateManager: func(o client.Object) resources.Manager {
					return tt.manager
				},
			}
			got, gotErr := r.CreateOrUpdate(context.Background(), tt.obj, tt.pc, tt.clusters)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("CreateOrUpdate() failed: %v", gotErr)
				}
				assert.Equal(t, gotErr.Error(), tt.obj.Status.Conditions[0].Message)
				return
			}
			if tt.wantErr {
				t.Fatal("CreateOrUpdate() succeeded unexpectedly")
			}
			if tt.wantStatusPhase != "" {
				assert.Equal(t, tt.wantStatusPhase, tt.obj.Status.Phase)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestVeleroReconciler_Delete(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		obj      *apiv1alpha1.Velero
		pc       *apiv1alpha1.ProviderConfig
		clusters spruntime.ClusterContext
		manager  resources.Manager
		want     ctrl.Result
		wantErr  bool
	}{
		{
			name: "managed objects deleted -> no error, status terminating",
			obj: createVeleroObj("v1", createPlugins(map[string]string{
				"aws": "v2",
			})),
			pc: &apiv1alpha1.ProviderConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
				Spec: apiv1alpha1.ProviderConfigSpec{
					PollInterval: &metav1.Duration{
						Duration: time.Minute,
					},
					ImagePullSecrets: []corev1.LocalObjectReference{},
					AvailableImages: []apiv1alpha1.AvailableVeleroImages{
						{
							Name:     "velero",
							Versions: []string{"v1"},
							Image:    "velero/velero",
						},
						{
							Name:     "aws",
							Versions: []string{"v2"},
							Image:    "velero/aws",
						},
					},
				},
			},
			clusters: fakeClusterContext(t),
			manager: fakeManager{
				results: []resources.Result{
					fakeResult(apiv1alpha1.Terminating, resources.OperationResultDeleted, resources.ManagedControlPlane, nil),
				},
			},
			want:    ctrl.Result{},
			wantErr: false,
		},
		{
			name: "managed objects not deleted -> requeue, status terminating",
			obj: createVeleroObj("v1", createPlugins(map[string]string{
				"aws": "v2",
			})),
			pc: &apiv1alpha1.ProviderConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
				Spec: apiv1alpha1.ProviderConfigSpec{
					PollInterval: &metav1.Duration{
						Duration: time.Minute,
					},
					ImagePullSecrets: []corev1.LocalObjectReference{},
					AvailableImages: []apiv1alpha1.AvailableVeleroImages{
						{
							Name:     "velero",
							Versions: []string{"v1"},
							Image:    "velero/velero",
						},
						{
							Name:     "aws",
							Versions: []string{"v2"},
							Image:    "velero/aws",
						},
					},
				},
			},
			clusters: fakeClusterContext(t),
			manager: fakeManager{
				results: []resources.Result{
					fakeResult(apiv1alpha1.Terminating, resources.OperationResultDeletionRequested, resources.ManagedControlPlane, nil),
				},
			},
			want: ctrl.Result{
				RequeueAfter: 5 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "managed objects with errors -> error",
			obj: createVeleroObj("v1", createPlugins(map[string]string{
				"aws": "v2",
			})),
			pc: &apiv1alpha1.ProviderConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
				Spec: apiv1alpha1.ProviderConfigSpec{
					PollInterval: &metav1.Duration{
						Duration: time.Minute,
					},
					ImagePullSecrets: []corev1.LocalObjectReference{},
					AvailableImages: []apiv1alpha1.AvailableVeleroImages{
						{
							Name:     "velero",
							Versions: []string{"v1"},
							Image:    "velero/velero",
						},
						{
							Name:     "aws",
							Versions: []string{"v2"},
							Image:    "velero/aws",
						},
					},
				},
			},
			clusters: fakeClusterContext(t),
			manager: fakeManager{
				results: []resources.Result{
					fakeResult(apiv1alpha1.Terminating, resources.OperationResultDeletionRequested, resources.ManagedControlPlane, errors.New("test")),
				},
			},
			want:    ctrl.Result{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &VeleroReconciler{
				OnboardingCluster: testutils.CreateFakeCluster(t, "onboarding", tt.obj),
				PlatformCluster:   testutils.CreateFakeCluster(t, "platform", tt.pc),
				PodNamespace:      "openmcp-system",
				CreateManager: func(o client.Object) resources.Manager {
					return tt.manager
				},
			}
			got, gotErr := r.Delete(context.Background(), tt.obj, tt.pc, tt.clusters)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("Delete() failed: %v", gotErr)
				}
				assert.Equal(t, gotErr.Error(), tt.obj.Status.Conditions[0].Message)
				return
			}
			if tt.wantErr {
				t.Fatal("Delete() succeeded unexpectedly")
			}
			assert.Equal(t, tt.want, got)
			assert.Equal(t, spruntime.StatusPhaseTerminating, tt.obj.Status.Phase)
		})
	}
}

func createVeleroObj(version string, plugins []apiv1alpha1.VeleroPlugin) *apiv1alpha1.Velero {
	return &apiv1alpha1.Velero{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
		Spec: apiv1alpha1.VeleroSpec{
			Version: version,
			Plugins: plugins,
		},
	}
}

func createPlugins(plugins map[string]string) []apiv1alpha1.VeleroPlugin {
	result := make([]apiv1alpha1.VeleroPlugin, 0, len(plugins))
	for k, v := range plugins {
		result = append(result, apiv1alpha1.VeleroPlugin{
			Name:    k,
			Version: v,
		})
	}
	return result
}

func createProviderConfig(availableImages []apiv1alpha1.AvailableVeleroImages) *apiv1alpha1.ProviderConfig {
	return &apiv1alpha1.ProviderConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
		Spec: apiv1alpha1.ProviderConfigSpec{
			PollInterval: &metav1.Duration{
				Duration: time.Minute,
			},
			ImagePullSecrets: []corev1.LocalObjectReference{},
			AvailableImages:  availableImages,
		},
	}
}

var _ resources.Manager = fakeManager{}
var _ resources.ManagedObject = fakeObject{}

type fakeObject struct {
	status resources.Status
}

// GetDeletionPolicy implements [resources.ManagedObject].
func (f fakeObject) GetDeletionPolicy() resources.DeletionPolicy {
	panic("unimplemented")
}

// GetDependencies implements [resources.ManagedObject].
func (f fakeObject) GetDependencies() []resources.ManagedObject {
	panic("unimplemented")
}

// GetObject implements [resources.ManagedObject].
func (f fakeObject) GetObject() client.Object {
	u := &unstructured.Unstructured{}
	u.SetName("test")
	u.SetNamespace("test")
	u.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "openmcp.cloud",
		Version: "v1alpha1",
		Kind:    "Test",
	})
	return u
}

// GetStatus implements [resources.ManagedObject].
func (f fakeObject) GetStatus(apiv1alpha1.ResourceLocation) resources.Status {
	return f.status
}

// Reconcile implements [resources.ManagedObject].
func (f fakeObject) Reconcile(ctx context.Context) error {
	panic("unimplemented")
}

type fakeManager struct {
	results []resources.Result
}

// AddCluster implements [resources.Manager].
func (f fakeManager) AddCluster(mc resources.ManagedCluster) {
}

// Apply implements [resources.Manager].
func (f fakeManager) Apply(context.Context) []resources.Result {
	return f.results
}

// Delete implements [resources.Manager].
func (f fakeManager) Delete(context.Context) []resources.Result {
	return f.results
}

func fakeClusterContext(t *testing.T) spruntime.ClusterContext {
	t.Helper()
	return spruntime.ClusterContext{
		MCPCluster:      testutils.CreateFakeCluster(t, "mcp"),
		WorkloadCluster: testutils.CreateFakeCluster(t, "workload"),
	}
}

func fakeResult(phase apiv1alpha1.InstancePhase, opResult controllerutil.OperationResult, clusterType resources.ClusterType, err error) resources.Result {
	return resources.Result{
		Object: fakeObject{
			status: resources.Status{
				Phase:    phase,
				Location: apiv1alpha1.ResourceLocation(clusterType),
			},
		},
		OperationResult: opResult,
		Cluster:         resources.NewManagedCluster(nil, nil, "", clusterType),
		Error:           err,
	}
}
