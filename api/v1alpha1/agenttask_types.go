/*
Copyright 2026 AgentTask Authors.

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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// AgentTaskSpec defines the desired state of AgentTask
type AgentTaskSpec struct {
	// RuntimeProfile selects the execution environment (e.g., "python3.11").
	// +required
	RuntimeProfile string `json:"runtimeProfile"`

	// Code defines the source code to execute.
	// +required
	Code CodeSource `json:"code"`

	// Resources allows specifying CPU/Memory requirements.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// TimeoutSeconds defines the maximum execution time.
	// +optional
	TimeoutSeconds int32 `json:"timeoutSeconds,omitempty"`

	// Backend selects the execution backend ("auto", "pod", "sandbox").
	// +optional
	Backend string `json:"backend,omitempty"`

	// Tenant identifies the user or group for multi-tenancy.
	// +optional
	Tenant string `json:"tenant,omitempty"`

	// Canceled indicates if the task should be cancelled.
	// +optional
	Canceled bool `json:"canceled,omitempty"`
}

// CodeSource defines where to find the code.
type CodeSource struct {
	// Source is the inline code content.
	// +optional
	Source string `json:"source,omitempty"`

	// ConfigMapRef references a ConfigMap containing the code.
	// +optional
	ConfigMapRef *corev1.ConfigMapKeySelector `json:"configMapRef,omitempty"`
}

// AgentTaskStatus defines the observed state of AgentTask.
type AgentTaskStatus struct {
	// Phase represents the current stage of the task lifecycle.
	// +optional
	Phase AgentTaskPhase `json:"phase,omitempty"`

	// PodRef references the created Pod or Sandbox.
	// +optional
	PodRef corev1.ObjectReference `json:"podRef,omitempty"`

	// Result stores the reference to the execution result.
	// +optional
	Result *ExecutionResult `json:"result,omitempty"`

	// Conditions store detailed status conditions.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ExitCode is the container exit code.
	// +optional
	ExitCode int32 `json:"exitCode,omitempty"`

	// Reason facilitates machine-readable failure analysis.
	// +optional
	Reason string `json:"reason,omitempty"`

	// Message provides a human-readable detailed status message.
	// +optional
	Message string `json:"message,omitempty"`

	// StartTime is when the task was accepted.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime is when the task reached a terminal state.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
}

// AgentTaskPhase describes the lifecycle phase.
// +kubebuilder:validation:Enum=Pending;Scheduled;Running;Succeeded;Failed;Timeout;Canceled
type AgentTaskPhase string

const (
	AgentTaskPhasePending   AgentTaskPhase = "Pending"
	AgentTaskPhaseScheduled AgentTaskPhase = "Scheduled"
	AgentTaskPhaseRunning   AgentTaskPhase = "Running"
	AgentTaskPhaseSucceeded AgentTaskPhase = "Succeeded"
	AgentTaskPhaseFailed    AgentTaskPhase = "Failed"
	AgentTaskPhaseTimeout   AgentTaskPhase = "Timeout"
	AgentTaskPhaseCanceled  AgentTaskPhase = "Canceled"
)

// ExecutionResult holds the structured result or reference.
type ExecutionResult struct {
	// JSON is the raw JSON result (if small).
	// +optional
	JSON string `json:"json,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// AgentTask is the Schema for the agenttasks API
type AgentTask struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of AgentTask
	// +required
	Spec AgentTaskSpec `json:"spec"`

	// status defines the observed state of AgentTask
	// +optional
	Status AgentTaskStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// AgentTaskList contains a list of AgentTask
type AgentTaskList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AgentTask `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AgentTask{}, &AgentTaskList{})
}
