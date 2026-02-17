/*
Copyright (c) 2026 hortator-ai
SPDX-License-Identifier: MIT
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AgentTaskPhase represents the current phase of the task
type AgentTaskPhase string

const (
	AgentTaskPhasePending        AgentTaskPhase = "Pending"
	AgentTaskPhaseRunning        AgentTaskPhase = "Running"
	AgentTaskPhaseWaiting        AgentTaskPhase = "Waiting"
	AgentTaskPhaseCompleted      AgentTaskPhase = "Completed"
	AgentTaskPhaseFailed         AgentTaskPhase = "Failed"
	AgentTaskPhaseBudgetExceeded AgentTaskPhase = "BudgetExceeded"
	AgentTaskPhaseTimedOut       AgentTaskPhase = "TimedOut"
	AgentTaskPhaseCancelled      AgentTaskPhase = "Cancelled"
	AgentTaskPhaseRetrying       AgentTaskPhase = "Retrying"
)

// ModelSpec defines the LLM endpoint configuration.
type ModelSpec struct {
	// Endpoint is the base URL (e.g. http://ollama:11434/v1, https://api.anthropic.com/v1)
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// Name is the model name (e.g. claude-sonnet, llama3:70b)
	// +optional
	Name string `json:"name,omitempty"`

	// ApiKeyRef is a reference to a K8s Secret containing the API key.
	// +optional
	ApiKeyRef *SecretKeyRef `json:"apiKeyRef,omitempty"`
}

// SecretKeyRef references a key in a Kubernetes Secret.
type SecretKeyRef struct {
	// SecretName is the name of the secret.
	// +kubebuilder:validation:Required
	SecretName string `json:"secretName"`

	// Key is the key within the secret.
	// +kubebuilder:validation:Required
	Key string `json:"key"`
}

// RetrySpec defines retry behavior for transient failures.
type RetrySpec struct {
	// MaxAttempts is the maximum number of retry attempts (0 = no retry).
	// +kubebuilder:default=0
	// +optional
	MaxAttempts int `json:"maxAttempts,omitempty"`

	// BackoffSeconds is the initial backoff duration (doubles each attempt).
	// +kubebuilder:default=30
	// +optional
	BackoffSeconds int `json:"backoffSeconds,omitempty"`

	// MaxBackoffSeconds caps the exponential backoff.
	// +kubebuilder:default=300
	// +optional
	MaxBackoffSeconds int `json:"maxBackoffSeconds,omitempty"`
}

// AttemptRecord tracks a single execution attempt.
type AttemptRecord struct {
	// Attempt number (1-indexed).
	Attempt int `json:"attempt"`

	// StartTime of this attempt.
	StartTime metav1.Time `json:"startTime"`

	// EndTime of this attempt.
	// +optional
	EndTime *metav1.Time `json:"endTime,omitempty"`

	// ExitCode of the agent container.
	// +optional
	ExitCode *int32 `json:"exitCode,omitempty"`

	// Reason for the attempt outcome.
	// +optional
	Reason string `json:"reason,omitempty"`
}

// BudgetSpec defines token/cost limits for a task.
type BudgetSpec struct {
	// MaxTokens is the total token cap (input + output).
	// +optional
	MaxTokens *int64 `json:"maxTokens,omitempty"`

	// MaxCostUsd is the dollar cap as string (e.g. "0.50").
	// +optional
	MaxCostUsd string `json:"maxCostUsd,omitempty"`
}

// StorageSpec defines storage configuration overrides.
type StorageSpec struct {
	// Retain exempts PVC from TTL cleanup.
	// +kubebuilder:default=false
	// +optional
	Retain bool `json:"retain,omitempty"`

	// RetainDays is a custom TTL override in days.
	// +optional
	RetainDays *int `json:"retainDays,omitempty"`

	// StorageClass is the Kubernetes storage class name.
	// +optional
	StorageClass string `json:"storageClass,omitempty"`

	// Size is the PVC size (e.g. "1Gi"). Only applies to tribune/centurion tiers.
	// +kubebuilder:default="1Gi"
	// +optional
	Size string `json:"size,omitempty"`
}

// StuckDetectionSpec defines stuck detection parameters.
type StuckDetectionSpec struct {
	// +optional
	ToolDiversityMin *float64 `json:"toolDiversityMin,omitempty"`

	// +optional
	MaxRepeatedPrompts *int `json:"maxRepeatedPrompts,omitempty"`

	// +optional
	StatusStaleMinutes *int `json:"statusStaleMinutes,omitempty"`

	// +kubebuilder:validation:Enum=warn;kill;escalate
	// +optional
	Action string `json:"action,omitempty"`
}

// HealthSpec defines per-task health/stuck detection overrides.
type HealthSpec struct {
	// +optional
	StuckDetection *StuckDetectionSpec `json:"stuckDetection,omitempty"`
}

// PresidioSpec defines per-task Presidio overrides.
type PresidioSpec struct {
	// ConfigRef is the name of a ConfigMap with custom Presidio config.
	// +optional
	ConfigRef string `json:"configRef,omitempty"`

	// +optional
	ScoreThreshold *float64 `json:"scoreThreshold,omitempty"`

	// +kubebuilder:validation:Enum=redact;detect;hash;mask
	// +optional
	Action string `json:"action,omitempty"`
}

// EnvVarSource represents a source for the value of an EnvVar.
type EnvVarSource struct {
	// +optional
	SecretKeyRef *SecretKeyRef `json:"secretKeyRef,omitempty"`
}

// EnvVar represents an environment variable.
type EnvVar struct {
	// Name of the environment variable.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Value of the environment variable.
	// +optional
	Value string `json:"value,omitempty"`

	// ValueFrom is a source for the environment variable's value.
	// +optional
	ValueFrom *EnvVarSource `json:"valueFrom,omitempty"`
}

// ResourceList defines cpu and memory quantities.
type ResourceList struct {
	// +optional
	CPU string `json:"cpu,omitempty"`

	// +optional
	Memory string `json:"memory,omitempty"`
}

// ResourceRequirements defines resource requests/limits.
type ResourceRequirements struct {
	// +optional
	Requests *ResourceList `json:"requests,omitempty"`

	// +optional
	Limits *ResourceList `json:"limits,omitempty"`
}

// TokenUsage tracks token consumption.
type TokenUsage struct {
	// +optional
	Input int64 `json:"input,omitempty"`

	// +optional
	Output int64 `json:"output,omitempty"`
}

// AgentTaskSpec defines the desired state of AgentTask
type AgentTaskSpec struct {
	// Prompt is the task instruction for the agent.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Prompt string `json:"prompt"`

	// Role is a reference to an AgentRole or ClusterAgentRole by name.
	// +optional
	Role string `json:"role,omitempty"`

	// Flavor is a free-form addendum appended to the role's rules.
	// +optional
	Flavor string `json:"flavor,omitempty"`

	// Tier is the agent hierarchy tier.
	// +kubebuilder:validation:Enum=tribune;centurion;legionary
	// +kubebuilder:default=legionary
	// +optional
	Tier string `json:"tier,omitempty"`

	// ParentTaskID is the task ID of the parent task.
	// +optional
	ParentTaskID string `json:"parentTaskId,omitempty"`

	// Model defines the LLM endpoint configuration.
	// +optional
	Model *ModelSpec `json:"model,omitempty"`

	// ThinkingLevel is the reasoning depth hint.
	// +kubebuilder:validation:Enum=low;medium;high
	// +optional
	ThinkingLevel string `json:"thinkingLevel,omitempty"`

	// Image is the container image to use for the agent.
	// +optional
	Image string `json:"image,omitempty"`

	// Capabilities are the permissions/tools available to the agent.
	// +optional
	Capabilities []string `json:"capabilities,omitempty"`

	// Env is the list of environment variables to inject into the agent pod.
	// +optional
	Env []EnvVar `json:"env,omitempty"`

	// Timeout is the task timeout in seconds.
	// +kubebuilder:default=600
	// +optional
	Timeout *int `json:"timeout,omitempty"`

	// Budget defines token/cost limits for this task.
	// +optional
	Budget *BudgetSpec `json:"budget,omitempty"`

	// HierarchyBudget is a shared budget pool across the entire task tree.
	// Only meaningful on root tasks (no parent). Children inherit and decrement
	// from the root's hierarchy budget.
	// +optional
	HierarchyBudget *BudgetSpec `json:"hierarchyBudget,omitempty"`

	// Resources defines compute resources for the agent pod.
	// +optional
	Resources *ResourceRequirements `json:"resources,omitempty"`

	// Storage defines storage configuration overrides.
	// +optional
	Storage *StorageSpec `json:"storage,omitempty"`

	// Health defines per-task health/stuck detection overrides.
	// +optional
	Health *HealthSpec `json:"health,omitempty"`

	// Presidio defines per-task Presidio overrides.
	// +optional
	Presidio *PresidioSpec `json:"presidio,omitempty"`

	// Retry defines retry behavior for transient failures.
	// +optional
	Retry *RetrySpec `json:"retry,omitempty"`

	// MaxIterations is the maximum number of planning loop iterations.
	// Tribunes default to 5, centurions to 3, legionaries to 1.
	// Set to 1 to disable the planning loop (single-shot execution).
	// +optional
	MaxIterations *int `json:"maxIterations,omitempty"`

	// ExitCriteria is a human-readable description of when this task is complete.
	// Injected into the agent's system prompt to guide self-assessment.
	// E.g. "All tests pass with exit code 0" or "Report contains all 5 sections".
	// +optional
	ExitCriteria string `json:"exitCriteria,omitempty"`

	// InputFiles are files delivered to /inbox when the task starts.
	// Files are base64-encoded and written by the init container.
	// Size limit: ~1MB total (etcd constraint).
	// +optional
	InputFiles []InputFile `json:"inputFiles,omitempty"`
}

// InputFile represents a file to deliver to the agent's /inbox directory.
type InputFile struct {
	// Filename is the name of the file (e.g. "data.csv").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Filename string `json:"filename"`

	// Data is the base64-encoded file content.
	// +kubebuilder:validation:Required
	Data string `json:"data"`
}

// AgentTaskStatus defines the observed state of AgentTask
type AgentTaskStatus struct {
	// Phase is the current phase of the task.
	// +kubebuilder:validation:Enum=Pending;Running;Waiting;Completed;Failed;BudgetExceeded;TimedOut;Cancelled;Retrying
	// +optional
	Phase AgentTaskPhase `json:"phase,omitempty"`

	// Output contains the result/output from the agent.
	// +optional
	Output string `json:"output,omitempty"`

	// PodName is the name of the pod running this task.
	// +optional
	PodName string `json:"podName,omitempty"`

	// StartedAt is when the task started executing.
	// +optional
	StartedAt *metav1.Time `json:"startedAt,omitempty"`

	// CompletedAt is when the task finished.
	// +optional
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`

	// Duration is the human-readable duration of the task.
	// +optional
	Duration string `json:"duration,omitempty"`

	// TokensUsed tracks token consumption.
	// +optional
	TokensUsed *TokenUsage `json:"tokensUsed,omitempty"`

	// EstimatedCostUsd is the estimated cost in USD.
	// +optional
	EstimatedCostUsd string `json:"estimatedCostUsd,omitempty"`

	// HierarchyTokensUsed tracks cumulative token usage across the entire task tree.
	// Only populated on root tasks that define a HierarchyBudget.
	// +optional
	HierarchyTokensUsed *TokenUsage `json:"hierarchyTokensUsed,omitempty"`

	// HierarchyCostUsed tracks cumulative cost across the entire task tree.
	// Only populated on root tasks that define a HierarchyBudget.
	// +optional
	HierarchyCostUsed string `json:"hierarchyCostUsed,omitempty"`

	// ChildTasks are the task IDs of spawned children.
	// +optional
	ChildTasks []string `json:"childTasks,omitempty"`

	// PendingChildren tracks children the task is waiting on (set when entering Waiting phase).
	// +optional
	PendingChildren []string `json:"pendingChildren,omitempty"`

	// LastReincarnatedAt records when the task was last reincarnated.
	// Used to filter out stale children from prior iterations during child discovery.
	// +optional
	LastReincarnatedAt *metav1.Time `json:"lastReincarnatedAt,omitempty"`

	// Message provides human-readable status information.
	// +optional
	Message string `json:"message,omitempty"`

	// Attempts is the number of execution attempts so far.
	// +optional
	Attempts int `json:"attempts,omitempty"`

	// NextRetryTime is when the next retry should be attempted.
	// +optional
	NextRetryTime *metav1.Time `json:"nextRetryTime,omitempty"`

	// History records each execution attempt.
	// +optional
	History []AttemptRecord `json:"history,omitempty"`

	// Conditions represent the latest available observations.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Role",type=string,JSONPath=`.spec.role`
// +kubebuilder:printcolumn:name="Tier",type=string,JSONPath=`.spec.tier`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="Duration",type=string,JSONPath=`.status.duration`

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
