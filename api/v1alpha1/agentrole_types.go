/*
Copyright (c) 2026 GeneClackman
SPDX-License-Identifier: MIT
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AgentRoleSpec defines the desired state of AgentRole and ClusterAgentRole.
type AgentRoleSpec struct {
	// DefaultModel is the model name (e.g. claude-sonnet-4-20250514).
	// +optional
	DefaultModel string `json:"defaultModel,omitempty"`

	// DefaultEndpoint is the base URL for the LLM API.
	// +optional
	DefaultEndpoint string `json:"defaultEndpoint,omitempty"`

	// ApiKeyRef is a reference to a K8s Secret containing the API key.
	// +optional
	ApiKeyRef *SecretKeyRef `json:"apiKeyRef,omitempty"`

	// Tools is a list of tool names available to this role.
	// +optional
	Tools []string `json:"tools,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="Model",type=string,JSONPath=`.spec.defaultModel`
// +kubebuilder:printcolumn:name="Endpoint",type=string,JSONPath=`.spec.defaultEndpoint`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AgentRole is the Schema for the agentroles API
type AgentRole struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec AgentRoleSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// AgentRoleList contains a list of AgentRole
type AgentRoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AgentRole `json:"items"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Model",type=string,JSONPath=`.spec.defaultModel`
// +kubebuilder:printcolumn:name="Endpoint",type=string,JSONPath=`.spec.defaultEndpoint`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ClusterAgentRole is the cluster-scoped Schema for agent roles
type ClusterAgentRole struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec AgentRoleSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterAgentRoleList contains a list of ClusterAgentRole
type ClusterAgentRoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterAgentRole `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AgentRole{}, &AgentRoleList{})
	SchemeBuilder.Register(&ClusterAgentRole{}, &ClusterAgentRoleList{})
}
