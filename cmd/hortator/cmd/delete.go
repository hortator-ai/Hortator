/*
Copyright (c) 2026 hortator-ai
SPDX-License-Identifier: MIT
*/

package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "github.com/hortator-ai/Hortator/api/v1alpha1"
)

var (
	deleteForce bool
	deleteAll   bool
)

var deleteCmd = &cobra.Command{
	Use:     "delete <task-name>",
	Aliases: []string{"rm"},
	Short:   "Delete an agent task",
	Long: `Delete an agent task and its associated resources.

Examples:
  hortator delete my-task
  hortator delete my-task --force
  hortator delete --all`,
	RunE: runDelete,
}

func init() {
	deleteCmd.Flags().BoolVarP(&deleteForce, "force", "f", false, "Skip confirmation")
	deleteCmd.Flags().BoolVar(&deleteAll, "all", false, "Delete all tasks")
	rootCmd.AddCommand(deleteCmd)
}

func runDelete(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	if deleteAll {
		return deleteAllTasks(ctx)
	}

	if len(args) == 0 {
		return fmt.Errorf("task name required (or use --all)")
	}

	return deleteTask(ctx, args[0])
}

func deleteTask(ctx context.Context, name string) error {
	task := &corev1alpha1.AgentTask{}
	if err := k8sClient.Get(ctx, client.ObjectKey{
		Namespace: getNamespace(),
		Name:      name,
	}, task); err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}

	if !deleteForce {
		fmt.Printf("Delete task '%s'? [y/N]: ", name)
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("Cancelled")
			return nil
		}
	}

	if err := k8sClient.Delete(ctx, task); err != nil {
		return fmt.Errorf("failed to delete task: %w", err)
	}

	if outputFormat == "json" {
		data, _ := json.MarshalIndent(map[string]string{"task": name, "status": "deleted"}, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("✓ Task '%s' deleted\n", name)
	return nil
}

func deleteAllTasks(ctx context.Context) error {
	taskList := &corev1alpha1.AgentTaskList{}
	if err := k8sClient.List(ctx, taskList, client.InNamespace(getNamespace())); err != nil {
		return fmt.Errorf("failed to list tasks: %w", err)
	}

	if len(taskList.Items) == 0 {
		fmt.Printf("No tasks found in namespace '%s'\n", getNamespace())
		return nil
	}

	if !deleteForce {
		fmt.Printf("Delete all %d tasks in namespace '%s'? [y/N]: ", len(taskList.Items), getNamespace())
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("Cancelled")
			return nil
		}
	}

	for _, task := range taskList.Items {
		if err := k8sClient.Delete(ctx, &task); err != nil {
			fmt.Printf("✗ Failed to delete '%s': %v\n", task.Name, err)
		} else {
			fmt.Printf("✓ Deleted '%s'\n", task.Name)
		}
	}

	return nil
}
