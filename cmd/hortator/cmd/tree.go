/*
Copyright (c) 2026 GeneClackman
SPDX-License-Identifier: MIT
*/

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "github.com/hortator-ai/Hortator/api/v1alpha1"
)

var treeCmd = &cobra.Command{
	Use:   "tree <task-name>",
	Short: "Display task hierarchy as a tree",
	Long: `Display an agent task and all its descendants as an ASCII tree.

Examples:
  hortator tree fix-api
  hortator tree fix-api -o json`,
	Args: cobra.ExactArgs(1),
	RunE: runTree,
}

func init() {
	rootCmd.AddCommand(treeCmd)
}

type treeNode struct {
	Name     string      `json:"name"`
	Tier     string      `json:"tier"`
	Phase    string      `json:"phase"`
	Duration string      `json:"duration"`
	Children []*treeNode `json:"children,omitempty"`
}

func runTree(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	taskName := args[0]

	// Fetch root task
	root := &corev1alpha1.AgentTask{}
	if err := k8sClient.Get(ctx, client.ObjectKey{
		Namespace: getNamespace(),
		Name:      taskName,
	}, root); err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}

	// Fetch all tasks in namespace to build the tree
	taskList := &corev1alpha1.AgentTaskList{}
	if err := k8sClient.List(ctx, taskList, client.InNamespace(getNamespace())); err != nil {
		return fmt.Errorf("failed to list tasks: %w", err)
	}

	// Build parent->children map
	childMap := make(map[string][]corev1alpha1.AgentTask)
	for _, t := range taskList.Items {
		if t.Spec.ParentTaskID != "" {
			childMap[t.Spec.ParentTaskID] = append(childMap[t.Spec.ParentTaskID], t)
		}
	}

	// Build tree recursively
	node := buildTreeNode(root, childMap)

	if outputFormat == "json" {
		data, err := json.MarshalIndent(node, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	// Render ASCII tree
	printTreeNode(node, "", true)
	return nil
}

func buildTreeNode(task *corev1alpha1.AgentTask, childMap map[string][]corev1alpha1.AgentTask) *treeNode {
	node := &treeNode{
		Name:     task.Name,
		Tier:     capitalize(task.Spec.Tier),
		Phase:    string(task.Status.Phase),
		Duration: task.Status.Duration,
	}
	for _, child := range childMap[task.Name] {
		c := child // capture
		node.Children = append(node.Children, buildTreeNode(&c, childMap))
	}
	return node
}

func printTreeNode(node *treeNode, prefix string, isRoot bool) {
	label := fmt.Sprintf("%s (%s, %s, %s)", node.Name, node.Tier, node.Phase, node.Duration)

	if isRoot {
		fmt.Println(label)
	} else {
		fmt.Println(label)
	}

	for i, child := range node.Children {
		isLast := i == len(node.Children)-1
		var connector, childPrefix string
		if isLast {
			connector = "└── "
			childPrefix = "    "
		} else {
			connector = "├── "
			childPrefix = "│   "
		}

		childLabel := fmt.Sprintf("%s (%s, %s, %s)", child.Name, child.Tier, child.Phase, child.Duration)
		fmt.Printf("%s%s%s\n", prefix, connector, childLabel)

		if len(child.Children) > 0 {
			printSubTree(child, prefix+childPrefix)
		}
	}
}

func printSubTree(node *treeNode, prefix string) {
	for i, child := range node.Children {
		isLast := i == len(node.Children)-1
		var connector, childPrefix string
		if isLast {
			connector = "└── "
			childPrefix = "    "
		} else {
			connector = "├── "
			childPrefix = "│   "
		}

		childLabel := fmt.Sprintf("%s (%s, %s, %s)", child.Name, child.Tier, child.Phase, child.Duration)
		fmt.Printf("%s%s%s\n", prefix, connector, childLabel)

		if len(child.Children) > 0 {
			printSubTree(child, prefix+childPrefix)
		}
	}
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
