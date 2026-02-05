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
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"

	apiv1alpha1 "github.com/openmcp-project/service-provider-velero/api/v1alpha1"
	spruntime "github.com/openmcp-project/service-provider-velero/pkg/runtime"
	"github.com/openmcp-project/service-provider-velero/pkg/testutils"
)

func TestVeleroReconciler_CreateOrUpdate(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		obj      *apiv1alpha1.Velero
		pc       *apiv1alpha1.ProviderConfig
		clusters spruntime.ClusterContext
		want     ctrl.Result
		wantErr  bool
	}{
		{
			name: "Successful createOrUpdate",
			obj: &apiv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
				},
				Spec: apiv1alpha1.VeleroSpec{
					Version: "v1",
					Plugins: []apiv1alpha1.VeleroPlugin{
						{
							Name:    "aws",
							Version: "v2",
						},
					},
				},
			},
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
			clusters: spruntime.ClusterContext{
				MCPCluster:      testutils.CreateFakeCluster(t, "mcp").WithRESTConfig(&rest.Config{}),
				WorkloadCluster: testutils.CreateFakeCluster(t, "workload"),
			},
			want:    ctrl.Result{},
			wantErr: false,
		},
		{
			name: "Velero version not available",
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
			clusters: spruntime.ClusterContext{
				MCPCluster:      testutils.CreateFakeCluster(t, "mcp").WithRESTConfig(&rest.Config{}),
				WorkloadCluster: testutils.CreateFakeCluster(t, "workload"),
			},
			want:    ctrl.Result{},
			wantErr: true,
		},
		{
			name: "Plugin version not available",
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
			clusters: spruntime.ClusterContext{
				MCPCluster:      testutils.CreateFakeCluster(t, "mcp").WithRESTConfig(&rest.Config{}),
				WorkloadCluster: testutils.CreateFakeCluster(t, "workload"),
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
			}
			_, gotErr := r.CreateOrUpdate(context.Background(), tt.obj, tt.pc, tt.clusters)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("CreateOrUpdate() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("CreateOrUpdate() succeeded unexpectedly")
			}
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
		want     ctrl.Result
		wantErr  bool
	}{
		{
			name: "test delete",
			obj: &apiv1alpha1.Velero{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
				},
				Spec: apiv1alpha1.VeleroSpec{
					Version: "v1",
					Plugins: []apiv1alpha1.VeleroPlugin{
						{
							Name:    "aws",
							Version: "v2",
						},
					},
				},
			},
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
			clusters: spruntime.ClusterContext{
				MCPCluster:      testutils.CreateFakeCluster(t, "mcp"),
				WorkloadCluster: testutils.CreateFakeCluster(t, "workload"),
			},
			want:    ctrl.Result{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &VeleroReconciler{
				OnboardingCluster: testutils.CreateFakeCluster(t, "onboarding", tt.obj),
				PlatformCluster:   testutils.CreateFakeCluster(t, "platform", tt.pc),
				PodNamespace:      "openmcp-system",
			}
			_, gotErr := r.Delete(context.Background(), tt.obj, tt.pc, tt.clusters)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("Delete() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("Delete() succeeded unexpectedly")
			}
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
