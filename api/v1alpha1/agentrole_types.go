/*
Copyright (c) 2026 hortator-ai
SPDX-License-Identifier: MIT
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AgentRoleSpec defines the desired state of AgentRole and ClusterAgentRole.
type AgentRoleSpec struct {
	// TierAffinity is the hierarchy tier this role is designed for (e.g. centurion, legionary).
	// +optional
	TierAffinity string `json:"tierAffinity,omitempty"`

	// Description is a human-readable description of the role's purpose.
	// +optional
	Description string `json:"description,omitempty"`

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

	// Rules is a list of behavioral rules/guidelines for this role.
	// +optional
	Rules []string `json:"rules,omitempty"`

	// AntiPatterns is a list of things the role should avoid doing.
	// +optional
	AntiPatterns []string `json:"antiPatterns,omitempty"`

	// Health defines per-role health/stuck detection overrides.
	// These sit between the cluster defaults and per-task overrides in the cascade:
	// ConfigMap defaults -> AgentRole -> AgentTask (most specific wins).
	// +optional
	Health *HealthSpec `json:"health,omitempty"`
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
