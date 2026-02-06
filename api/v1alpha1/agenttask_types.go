/*
Copyright 2026.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AgentTaskPhase represents the current phase of the task
type AgentTaskPhase string

const (
	// AgentTaskPhasePending means the task has been accepted but not yet started
	AgentTaskPhasePending AgentTaskPhase = "Pending"
	// AgentTaskPhaseRunning means the task is currently executing
	AgentTaskPhaseRunning AgentTaskPhase = "Running"
	// AgentTaskPhaseSucceeded means the task completed successfully
	AgentTaskPhaseSucceeded AgentTaskPhase = "Succeeded"
	// AgentTaskPhaseFailed means the task failed
	AgentTaskPhaseFailed AgentTaskPhase = "Failed"
)

// AgentTaskSpec defines the desired state of AgentTask
type AgentTaskSpec struct {
	// Prompt is the task instruction for the agent
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Prompt string `json:"prompt"`

	// Capabilities are the permissions/tools available to the agent
	// +optional
	Capabilities []string `json:"capabilities,omitempty"`

	// Timeout is the maximum duration for task execution (e.g., "30m", "1h")
	// +kubebuilder:default="30m"
	// +optional
	Timeout string `json:"timeout,omitempty"`

	// Image is the container image to use for the agent
	// +kubebuilder:default="ghcr.io/hortator-ai/agent:latest"
	// +optional
	Image string `json:"image,omitempty"`

	// Model is the LLM model to use (e.g., "gpt-4", "claude-3")
	// +optional
	Model string `json:"model,omitempty"`

	// Environment variables to inject into the agent pod
	// +optional
	Env map[string]string `json:"env,omitempty"`

	// Resources defines compute resources for the agent pod
	// +optional
	Resources *ResourceRequirements `json:"resources,omitempty"`
}

// ResourceRequirements defines resource requests/limits
type ResourceRequirements struct {
	// CPU request (e.g., "100m", "1")
	// +optional
	CPU string `json:"cpu,omitempty"`

	// Memory request (e.g., "128Mi", "1Gi")
	// +optional
	Memory string `json:"memory,omitempty"`
}

// AgentTaskStatus defines the observed state of AgentTask
type AgentTaskStatus struct {
	// Phase is the current phase of the task
	// +kubebuilder:validation:Enum=Pending;Running;Succeeded;Failed
	// +optional
	Phase AgentTaskPhase `json:"phase,omitempty"`

	// Output contains the result/output from the agent
	// +optional
	Output string `json:"output,omitempty"`

	// PodName is the name of the pod running this task
	// +optional
	PodName string `json:"podName,omitempty"`

	// StartTime is when the task started executing
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime is when the task finished
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// Message provides human-readable status information
	// +optional
	Message string `json:"message,omitempty"`

	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="Pod",type=string,JSONPath=`.status.podName`

// AgentTask is the Schema for the agenttasks API
type AgentTask struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentTaskSpec   `json:"spec,omitempty"`
	Status AgentTaskStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AgentTaskList contains a list of AgentTask
type AgentTaskList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AgentTask `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AgentTask{}, &AgentTaskList{})
}
