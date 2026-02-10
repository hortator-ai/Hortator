/*
Copyright (c) 2026 GeneClackman
SPDX-License-Identifier: MIT
*/

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "github.com/michael-niemand/Hortator/api/v1alpha1"
)

var cancelForce bool

var cancelCmd = &cobra.Command{
	Use:   "cancel <task-name>",
	Short: "Cancel a pending or running agent task",
	Long: `Cancel an agent task by setting its phase to Cancelled.

Only tasks in Pending or Running phase can be cancelled. Tasks already in a
terminal state (Completed, Failed, Cancelled, BudgetExceeded, TimedOut) will
return an error.

Examples:
  hortator cancel my-task
  hortator cancel my-task --force`,
	Args: cobra.ExactArgs(1),
	RunE: runCancel,
}

func init() {
	cancelCmd.Flags().BoolVarP(&cancelForce, "force", "f", false, "Also delete the associated pod immediately")
	rootCmd.AddCommand(cancelCmd)
}

func runCancel(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	name := args[0]

	task := &corev1alpha1.AgentTask{}
	if err := k8sClient.Get(ctx, client.ObjectKey{
		Namespace: getNamespace(),
		Name:      name,
	}, task); err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}

	// Check if task is already in a terminal state
	switch task.Status.Phase {
	case corev1alpha1.AgentTaskPhaseCompleted,
		corev1alpha1.AgentTaskPhaseFailed,
		corev1alpha1.AgentTaskPhaseCancelled,
		corev1alpha1.AgentTaskPhaseBudgetExceeded,
		corev1alpha1.AgentTaskPhaseTimedOut:
		return fmt.Errorf("task already in terminal state: %s", task.Status.Phase)
	}

	// Update status to Cancelled
	now := metav1.NewTime(time.Now())
	task.Status.Phase = corev1alpha1.AgentTaskPhaseCancelled
	task.Status.CompletedAt = &now
	task.Status.Message = "Cancelled by user"

	if err := k8sClient.Status().Update(ctx, task); err != nil {
		return fmt.Errorf("failed to update task status: %w", err)
	}

	if outputFormat == "json" {
		data, _ := json.MarshalIndent(map[string]string{"task": name, "status": "cancelled"}, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("✓ Task '%s' cancelled\n", name)

	// If --force, delete the associated pod
	if cancelForce && task.Status.PodName != "" {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: getNamespace(),
				Name:      task.Status.PodName,
			},
		}
		if err := k8sClient.Delete(ctx, pod); err != nil {
			fmt.Printf("⚠ Failed to delete pod '%s': %v\n", task.Status.PodName, err)
		} else {
			fmt.Printf("✓ Pod '%s' deleted\n", task.Status.PodName)
		}
	}

	return nil
}
