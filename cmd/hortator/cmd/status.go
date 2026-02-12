/*
Copyright (c) 2026 GeneClackman
SPDX-License-Identifier: MIT
*/

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "github.com/hortator-ai/Hortator/api/v1alpha1"
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

	if outputFormat == "json" {
		result := map[string]interface{}{
			"name":      task.Name,
			"namespace": task.Namespace,
			"phase":     task.Status.Phase,
			"message":   task.Status.Message,
			"pod":       task.Status.PodName,
			"tier":      task.Spec.Tier,
			"prompt":    task.Spec.Prompt,
			"image":     task.Spec.Image,
		}
		if task.Status.StartedAt != nil {
			result["startedAt"] = task.Status.StartedAt.Time
		}
		if task.Status.CompletedAt != nil {
			result["completedAt"] = task.Status.CompletedAt.Time
		}
		if task.Status.Duration != "" {
			result["duration"] = task.Status.Duration
		}
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("Name:       %s\n", task.Name)
	fmt.Printf("Namespace:  %s\n", task.Namespace)
	fmt.Printf("Phase:      %s\n", task.Status.Phase)
	fmt.Printf("Message:    %s\n", task.Status.Message)
	fmt.Printf("Pod:        %s\n", task.Status.PodName)

	if task.Status.StartedAt != nil {
		fmt.Printf("Started:    %s\n", task.Status.StartedAt.Format(time.RFC3339))
	}
	if task.Status.CompletedAt != nil {
		fmt.Printf("Completed:  %s\n", task.Status.CompletedAt.Format(time.RFC3339))
	}
	if task.Status.Duration != "" {
		fmt.Printf("Duration:   %s\n", task.Status.Duration)
	}

	fmt.Println("\nSpec:")
	fmt.Printf("  Prompt:       %s\n", truncate(task.Spec.Prompt, 60))
	fmt.Printf("  Image:        %s\n", task.Spec.Image)
	if task.Spec.Timeout != nil {
		fmt.Printf("  Timeout:      %ds\n", *task.Spec.Timeout)
	}
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

	if outputFormat == "json" {
		var items []map[string]interface{}
		for _, task := range taskList.Items {
			item := map[string]interface{}{
				"name":    task.Name,
				"phase":   task.Status.Phase,
				"age":     time.Since(task.CreationTimestamp.Time).Round(time.Second).String(),
				"pod":     task.Status.PodName,
				"message": task.Status.Message,
			}
			items = append(items, item)
		}
		data, err := json.MarshalIndent(items, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	if len(taskList.Items) == 0 {
		fmt.Printf("No tasks found in namespace '%s'\n", getNamespace())
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tPHASE\tAGE\tPOD\tMESSAGE")

	for _, task := range taskList.Items {
		age := time.Since(task.CreationTimestamp.Time).Round(time.Second)
		message := truncate(task.Status.Message, 40)
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
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
