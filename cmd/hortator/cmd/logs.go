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
	"io"
	"strings"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "github.com/hortator-ai/Hortator/api/v1alpha1"
)

var (
	logsFollow bool
	logsTail   int64
)

var logsCmd = &cobra.Command{
	Use:   "logs <task-name>",
	Short: "View logs from an agent task",
	Long: `View the logs from an agent task's pod.

Examples:
  hortator logs my-task
  hortator logs my-task -f
  hortator logs my-task --tail 100`,
	Args: cobra.ExactArgs(1),
	RunE: runLogs,
}

func init() {
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Follow log output")
	logsCmd.Flags().Int64Var(&logsTail, "tail", -1, "Lines from end")
	rootCmd.AddCommand(logsCmd)
}

func runLogs(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	taskName := args[0]

	task := &corev1alpha1.AgentTask{}
	if err := k8sClient.Get(ctx, client.ObjectKey{
		Namespace: getNamespace(),
		Name:      taskName,
	}, task); err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}

	if task.Status.PodName == "" {
		return fmt.Errorf("task has no associated pod (phase: %s)", task.Status.Phase)
	}

	opts := &corev1.PodLogOptions{
		Container: "agent",
		Follow:    logsFollow,
	}
	if logsTail > 0 {
		opts.TailLines = &logsTail
	}

	req := clientset.CoreV1().Pods(getNamespace()).GetLogs(task.Status.PodName, opts)
	stream, err := req.Stream(ctx)
	if err != nil {
		return fmt.Errorf("failed to get logs: %w", err)
	}
	defer func() { _ = stream.Close() }()

	reader := bufio.NewReader(stream)

	if outputFormat == "json" {
		var sb strings.Builder
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					sb.WriteString(line)
					break
				}
				return fmt.Errorf("error reading logs: %w", err)
			}
			sb.WriteString(line)
		}
		data, _ := json.MarshalIndent(map[string]string{"task": taskName, "logs": sb.String()}, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("error reading logs: %w", err)
		}
		fmt.Print(line)
	}

	return nil
}
