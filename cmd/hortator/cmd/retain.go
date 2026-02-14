/*
Copyright (c) 2026 hortator-ai
SPDX-License-Identifier: MIT
*/

/*
The retain command allows agents to mark their PVC for retention after task
completion. By default, the operator garbage-collects agent PVCs when tasks
finish. Running `hortator retain` patches the annotation on the AgentTask,
telling the operator to keep the PVC around for debugging or artifact retrieval.

Usage inside an agent pod:

	hortator retain
	hortator retain --reason "contains training artifacts"
*/
package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "github.com/hortator-ai/Hortator/api/v1alpha1"
)

var retainCmd = &cobra.Command{
	Use:   "retain",
	Short: "Mark the task's PVC for retention after completion",
	Long: `Retain patches the AgentTask with annotation hortator.ai/retain-pvc=true,
preventing the operator from garbage-collecting the PVC when the task completes.

This is useful when agents produce large artifacts that need to be inspected
or copied out after the task finishes.

Examples:
  # Retain PVC with default reason
  hortator retain

  # Retain with a reason annotation
  hortator retain --reason "contains training checkpoints"`,
	RunE: runRetain,
}

var retainReason string

func init() {
	retainCmd.Flags().StringVar(&retainReason, "reason", "", "Optional reason for retaining the PVC")
	rootCmd.AddCommand(retainCmd)
}

func runRetain(cmd *cobra.Command, args []string) error {
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

	// Patch annotations
	if task.Annotations == nil {
		task.Annotations = map[string]string{}
	}
	task.Annotations["hortator.ai/retain-pvc"] = "true"
	if retainReason != "" {
		task.Annotations["hortator.ai/retain-reason"] = retainReason
	}

	if err := k8sClient.Update(ctx, task); err != nil {
		return fmt.Errorf("failed to update AgentTask annotations: %w", err)
	}

	fmt.Printf("[hortator] Marked PVC for retention on %s/%s\n", taskNamespace, taskName)
	if retainReason != "" {
		fmt.Printf("[hortator] Reason: %s\n", retainReason)
	}
	return nil
}
