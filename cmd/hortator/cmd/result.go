/*
Copyright 2026 Hortator Authors.

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

package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "github.com/michael-niemand/Hortator/api/v1alpha1"
)

var (
	resultJSON bool
	resultWait bool
)

var resultCmd = &cobra.Command{
	Use:   "result <task-name>",
	Short: "Get the result of a completed task",
	Long: `Get the result/output of a completed agent task.

Examples:
  hortator result my-task
  hortator result my-task --json
  hortator result my-task --wait`,
	Args: cobra.ExactArgs(1),
	RunE: runResult,
}

func init() {
	resultCmd.Flags().BoolVar(&resultJSON, "json", false, "Output as JSON")
	resultCmd.Flags().BoolVarP(&resultWait, "wait", "w", false, "Wait for completion")
	rootCmd.AddCommand(resultCmd)
}

func runResult(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	taskName := args[0]

	if resultWait {
		if err := waitForTask(ctx, taskName); err != nil {
			return err
		}
	}

	task := &corev1alpha1.AgentTask{}
	if err := k8sClient.Get(ctx, client.ObjectKey{
		Namespace: getNamespace(),
		Name:      taskName,
	}, task); err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}

	switch task.Status.Phase {
	case corev1alpha1.AgentTaskPhaseSucceeded, corev1alpha1.AgentTaskPhaseFailed:
	case corev1alpha1.AgentTaskPhasePending:
		return fmt.Errorf("task is still pending")
	case corev1alpha1.AgentTaskPhaseRunning:
		return fmt.Errorf("task is still running (use --wait)")
	default:
		return fmt.Errorf("unknown task phase: %s", task.Status.Phase)
	}

	if resultJSON {
		result := map[string]interface{}{
			"name":      task.Name,
			"namespace": task.Namespace,
			"phase":     task.Status.Phase,
			"message":   task.Status.Message,
			"output":    task.Status.Output,
		}
		if task.Status.StartTime != nil {
			result["startTime"] = task.Status.StartTime.Time
		}
		if task.Status.CompletionTime != nil {
			result["completionTime"] = task.Status.CompletionTime.Time
		}
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	if task.Status.Phase == corev1alpha1.AgentTaskPhaseFailed {
		fmt.Printf("Task failed: %s\n", task.Status.Message)
		return nil
	}

	if task.Status.Output == "" {
		fmt.Println("No output available")
		return nil
	}

	fmt.Println(task.Status.Output)
	return nil
}
