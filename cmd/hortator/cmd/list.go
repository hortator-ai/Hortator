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

	corev1alpha1 "github.com/michael-niemand/Hortator/api/v1alpha1"
)

var (
	listAllNamespaces bool
	listPhase         string
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List agent tasks",
	Long: `List agent tasks in the cluster.

Examples:
  hortator list
  hortator list -A
  hortator list --phase Running
  hortator list --json`,
	RunE: runList,
}

func init() {
	listCmd.Flags().BoolVarP(&listAllNamespaces, "all-namespaces", "A", false, "All namespaces")
	listCmd.Flags().StringVar(&listPhase, "phase", "", "Filter by phase")
	rootCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	taskList := &corev1alpha1.AgentTaskList{}

	var listOpts []client.ListOption
	if !listAllNamespaces {
		listOpts = append(listOpts, client.InNamespace(getNamespace()))
	}

	if err := k8sClient.List(ctx, taskList, listOpts...); err != nil {
		return fmt.Errorf("failed to list tasks: %w", err)
	}

	if listPhase != "" {
		var filtered []corev1alpha1.AgentTask
		for _, task := range taskList.Items {
			if string(task.Status.Phase) == listPhase {
				filtered = append(filtered, task)
			}
		}
		taskList.Items = filtered
	}

	if outputFormat == "json" {
		var items []map[string]interface{}
		for _, task := range taskList.Items {
			item := map[string]interface{}{
				"name":      task.Name,
				"namespace": task.Namespace,
				"phase":     task.Status.Phase,
				"age":       time.Since(task.CreationTimestamp.Time).Round(time.Second).String(),
			}
			if task.Status.PodName != "" {
				item["pod"] = task.Status.PodName
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
		if listAllNamespaces {
			fmt.Println("No tasks found")
		} else {
			fmt.Printf("No tasks found in namespace '%s'\n", getNamespace())
		}
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	if listAllNamespaces {
		_, _ = fmt.Fprintln(w, "NAMESPACE\tNAME\tPHASE\tAGE\tPOD")
		for _, task := range taskList.Items {
			age := time.Since(task.CreationTimestamp.Time).Round(time.Second)
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				task.Namespace, task.Name, task.Status.Phase, age, task.Status.PodName)
		}
	} else {
		_, _ = fmt.Fprintln(w, "NAME\tPHASE\tAGE\tPOD")
		for _, task := range taskList.Items {
			age := time.Since(task.CreationTimestamp.Time).Round(time.Second)
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				task.Name, task.Status.Phase, age, task.Status.PodName)
		}
	}

	return w.Flush()
}
