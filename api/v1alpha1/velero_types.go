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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// InstancePhase is a custom type representing the phase of a service instance.
type InstancePhase string

// Constants representing the phases of an instance lifecycle.
const (
	Pending     InstancePhase = "Pending"
	Progressing InstancePhase = "Progressing"
	Ready       InstancePhase = "Ready"
	Failed      InstancePhase = "Failed"
	Terminating InstancePhase = "Terminating"
	Unknown     InstancePhase = "Unknown"
)

// VeleroSpec defines the desired state of Velero
type VeleroSpec struct {
	// The Velero image to use.
	// +kubebuilder:default=velero/velero
	Image string `json:"image"`
	// The Velero version.
	Version string `json:"version"`
	// Plugins that should be installed.
	Plugins []VeleroPlugin `json:"plugins"`
}

// VeleroPlugin defines a velero plugin
type VeleroPlugin struct {
	// The Velero plugin image to use.
	Image string `json:"image"`
	// The Velero plugin version.
	Version string `json:"version"`
}

// VeleroStatus defines the observed state of Velero.
type VeleroStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the Velero resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// ObservedGeneration is the generation of this resource that was last reconciled by the controller.
	ObservedGeneration int64 `json:"observedGeneration"`
	// Phase is the current phase of the resource.
	Phase string `json:"phase"`
	// Resources managed by this velero instance
	// +optional
	Resources []ManagedResource `json:"resources"`
}

// ManagedResource defines a kubernetes object with its lifecycle phase
type ManagedResource struct {
	corev1.TypedObjectReference `json:",inline"`

	Phase   InstancePhase `json:"phase"`
	Message string        `json:"message,omitempty"`
}

// Velero is the Schema for the veleros API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:JSONPath=`.status.phase`,name="Phase",type=string
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:metadata:labels="openmcp.cloud/cluster=onboarding"
type Velero struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of Velero
	// +required
	Spec VeleroSpec `json:"spec"`

	// status defines the observed state of Velero
	// +optional
	Status VeleroStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// VeleroList contains a list of Velero
type VeleroList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Velero `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Velero{}, &VeleroList{})
}

// Finalizer returns the finalizer string for the Velero resource
func (o *Velero) Finalizer() string {
	return GroupVersion.Group + "/finalizer"
}

// GetStatus returns the status of the Velero resource
func (o *Velero) GetStatus() any {
	return o.Status
}

// GetConditions returns the conditions of the Velero resource
func (o *Velero) GetConditions() *[]metav1.Condition {
	return &o.Status.Conditions
}

// SetPhase sets the phase of the Velero resource status
func (o *Velero) SetPhase(phase string) {
	o.Status.Phase = phase
}

// SetObservedGeneration sets the observed generation of the Velero resource
func (o *Velero) SetObservedGeneration(gen int64) {
	o.Status.ObservedGeneration = gen
}
