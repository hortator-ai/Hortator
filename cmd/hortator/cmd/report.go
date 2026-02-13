/*
Copyright (c) 2026 hortator-ai
SPDX-License-Identifier: MIT
*/

/*
The report command allows agents to write results directly to their AgentTask
CRD status. This is the primary mechanism for returning results from agent
work — no stdout parsing, no file scraping, just a clean API call.

The agent's runtime calls:

	hortator report --result "Here's what I built" --tokens-in 500 --tokens-out 2000

This patches the AgentTask status with the result and token usage. The operator
and gateway watch the CRD and pick up the update instantly.

Artifacts (code files, patches, etc.) stay on the PVC at /outbox/artifacts/.
The --artifacts flag records their paths in the CRD for discoverability.
*/
package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "github.com/hortator-ai/Hortator/api/v1alpha1"
)

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Report task results back to the AgentTask CRD",
	Long: `Report writes the agent's result, token usage, and artifact manifest
directly to the AgentTask status via the Kubernetes API.

This is the standard way for agents to return results. The operator
and API gateway watch the CRD and receive updates instantly.

Large artifacts (code, patches, reports) should be written to
/outbox/artifacts/ on the PVC. Use --artifacts to record their paths.

Examples:
  # Report a simple text result
  hortator report --result "The answer is 42"

  # Report with token usage
  hortator report --result "Built the handler" --tokens-in 500 --tokens-out 2000

  # Report with artifact references
  hortator report --result "Implemented REST API" \
    --artifacts "artifacts/handler.go,artifacts/handler_test.go" \
    --tokens-in 1200 --tokens-out 3500`,
	RunE: runReport,
}

var (
	reportResult    string
	reportTokensIn  int64
	reportTokensOut int64
	reportArtifacts string
)

func init() {
	reportCmd.Flags().StringVar(&reportResult, "result", "", "Result summary text")
	reportCmd.Flags().Int64Var(&reportTokensIn, "tokens-in", 0, "Input tokens consumed")
	reportCmd.Flags().Int64Var(&reportTokensOut, "tokens-out", 0, "Output tokens consumed")
	reportCmd.Flags().StringVar(&reportArtifacts, "artifacts", "", "Comma-separated artifact paths (relative to /outbox/)")
	_ = reportCmd.MarkFlagRequired("result")
	rootCmd.AddCommand(reportCmd)
}

func runReport(cmd *cobra.Command, args []string) error {
	// Determine task name and namespace from environment
	// (set by the operator when creating the pod)
	taskName := os.Getenv("HORTATOR_TASK_NAME")
	taskNamespace := os.Getenv("HORTATOR_TASK_NAMESPACE")

	if taskName == "" {
		return fmt.Errorf("HORTATOR_TASK_NAME not set (are you running inside a Hortator agent pod?)")
	}
	if taskNamespace == "" {
		taskNamespace = getNamespace()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Fetch the current AgentTask
	task := &corev1alpha1.AgentTask{}
	if err := k8sClient.Get(ctx, client.ObjectKey{
		Namespace: taskNamespace,
		Name:      taskName,
	}, task); err != nil {
		return fmt.Errorf("failed to get AgentTask %s/%s: %w", taskNamespace, taskName, err)
	}

	// Update status fields
	task.Status.Output = reportResult

	if reportTokensIn > 0 || reportTokensOut > 0 {
		task.Status.TokensUsed = &corev1alpha1.TokenUsage{
			Input:  reportTokensIn,
			Output: reportTokensOut,
		}
	}

	// Record artifacts as a comma-separated annotation for now.
	// Future: add a proper artifacts field to AgentTaskStatus.
	if reportArtifacts != "" {
		if task.Annotations == nil {
			task.Annotations = map[string]string{}
		}
		task.Annotations["hortator.ai/artifacts"] = reportArtifacts
		if err := k8sClient.Update(ctx, task); err != nil {
			return fmt.Errorf("failed to update task annotations: %w", err)
		}
		// Re-fetch after annotation update to avoid conflict on status update
		if err := k8sClient.Get(ctx, client.ObjectKey{
			Namespace: taskNamespace,
			Name:      taskName,
		}, task); err != nil {
			return fmt.Errorf("failed to re-fetch AgentTask: %w", err)
		}
	}

	// Patch status
	if err := k8sClient.Status().Update(ctx, task); err != nil {
		return fmt.Errorf("failed to update AgentTask status: %w", err)
	}

	// Parse artifacts for logging
	var artifactList []string
	if reportArtifacts != "" {
		artifactList = strings.Split(reportArtifacts, ",")
	}

	fmt.Printf("[hortator] Reported result to %s/%s (tokens: %d in, %d out, %d artifacts)\n",
		taskNamespace, taskName, reportTokensIn, reportTokensOut, len(artifactList))
	return nil
}

// Ensure HORTATOR_TASK_NAMESPACE is used for namespace resolution
func init() {
	// Override namespace from env if inside a pod
	if ns := os.Getenv("HORTATOR_TASK_NAMESPACE"); ns != "" && namespace == "" {
		namespace = ns
	}
	_ = metav1.Now // ensure metav1 import used
}
