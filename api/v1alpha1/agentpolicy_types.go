/*
Copyright (c) 2026 hortator-ai
SPDX-License-Identifier: MIT
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AgentPolicySpec defines fine-grained restrictions for agent tasks in a namespace.
type AgentPolicySpec struct {
	// NamespaceSelector restricts which namespaces this policy applies to.
	// If empty, applies to the namespace it's created in.
	// +optional
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`

	// AllowedCapabilities is the whitelist of capabilities tasks can request.
	// If empty, all capabilities are denied.
	// +optional
	AllowedCapabilities []string `json:"allowedCapabilities,omitempty"`

	// DeniedCapabilities explicitly blocks specific capabilities (overrides allowed).
	// +optional
	DeniedCapabilities []string `json:"deniedCapabilities,omitempty"`

	// AllowedImages are glob patterns for permitted container images.
	// e.g., ["ghcr.io/hortator/*", "myregistry.com/agents/*"]
	// +optional
	AllowedImages []string `json:"allowedImages,omitempty"`

	// MaxBudget sets the maximum budget any single task can request.
	// +optional
	MaxBudget *BudgetSpec `json:"maxBudget,omitempty"`

	// MaxTimeout is the maximum timeout in seconds any task can set.
	// +optional
	MaxTimeout *int `json:"maxTimeout,omitempty"`

	// MaxTier is the highest tier allowed (legionary < centurion < tribune).
	// +kubebuilder:validation:Enum=legionary;centurion;tribune
	// +optional
	MaxTier string `json:"maxTier,omitempty"`

	// EgressAllowlist restricts outbound network access.
	// +optional
	EgressAllowlist []EgressRule `json:"egressAllowlist,omitempty"`

	// RequirePresidio forces PII scanning on all tasks.
	// +optional
	RequirePresidio bool `json:"requirePresidio,omitempty"`

	// MaxConcurrentTasks limits active tasks per namespace.
	// +optional
	MaxConcurrentTasks *int `json:"maxConcurrentTasks,omitempty"`
}

// EgressRule defines an allowed outbound network destination.
type EgressRule struct {
	// Host is a domain or IP CIDR (e.g., "api.openai.com", "10.0.0.0/8")
	Host string `json:"host"`

	// Ports allowed (empty = all ports)
	// +optional
	Ports []int `json:"ports,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:printcolumn:name="MaxTier",type=string,JSONPath=`.spec.maxTier`
// +kubebuilder:printcolumn:name="RequirePresidio",type=boolean,JSONPath=`.spec.requirePresidio`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AgentPolicy defines fine-grained restrictions for agent tasks in a namespace.
// When present, tasks in the namespace must comply with all matching policies.
type AgentPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec AgentPolicySpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// AgentPolicyList contains a list of AgentPolicy
type AgentPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AgentPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AgentPolicy{}, &AgentPolicyList{})
}
