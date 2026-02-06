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
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "github.com/michael-niemand/Hortator/api/v1alpha1"
)

var statusCmd = &cobra.Command{
	Use:   "status [task-name]",
	Short: "Get status of an agent task",
	Long: `Get the current status of an agent task.

Examples:
  hortator status my-task
  hortator status`,
	RunE: runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	if len(args) > 0 {
		return showTaskStatus(ctx, args[0])
	}
	return showAllTasks(ctx)
}

func showTaskStatus(ctx context.Context, name string) error {
	task := &corev1alpha1.AgentTask{}
	if err := k8sClient.Get(ctx, client.ObjectKey{
		Namespace: getNamespace(),
		Name:      name,
	}, task); err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}

	fmt.Printf("Name:       %s\n", task.Name)
	fmt.Printf("Namespace:  %s\n", task.Namespace)
	fmt.Printf("Phase:      %s\n", task.Status.Phase)
	fmt.Printf("Message:    %s\n", task.Status.Message)
	fmt.Printf("Pod:        %s\n", task.Status.PodName)

	if task.Status.StartTime != nil {
		fmt.Printf("Started:    %s\n", task.Status.StartTime.Format(time.RFC3339))
	}
	if task.Status.CompletionTime != nil {
		fmt.Printf("Completed:  %s\n", task.Status.CompletionTime.Format(time.RFC3339))
	}
	if task.Status.StartTime != nil && task.Status.CompletionTime != nil {
		duration := task.Status.CompletionTime.Sub(task.Status.StartTime.Time)
		fmt.Printf("Duration:   %s\n", duration.Round(time.Second))
	}

	fmt.Println("\nSpec:")
	fmt.Printf("  Prompt:       %s\n", truncate(task.Spec.Prompt, 60))
	fmt.Printf("  Image:        %s\n", task.Spec.Image)
	fmt.Printf("  Timeout:      %s\n", task.Spec.Timeout)
	if len(task.Spec.Capabilities) > 0 {
		fmt.Printf("  Capabilities: %v\n", task.Spec.Capabilities)
	}

	return nil
}

func showAllTasks(ctx context.Context) error {
	taskList := &corev1alpha1.AgentTaskList{}
	if err := k8sClient.List(ctx, taskList, client.InNamespace(getNamespace())); err != nil {
		return fmt.Errorf("failed to list tasks: %w", err)
	}

	if len(taskList.Items) == 0 {
		fmt.Printf("No tasks found in namespace '%s'\n", getNamespace())
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tPHASE\tAGE\tPOD\tMESSAGE")

	for _, task := range taskList.Items {
		age := time.Since(task.CreationTimestamp.Time).Round(time.Second)
		message := truncate(task.Status.Message, 40)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			task.Name, task.Status.Phase, age, task.Status.PodName, message)
	}

	return w.Flush()
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
