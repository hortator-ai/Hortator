/*
Copyright (c) 2026 hortator-ai
SPDX-License-Identifier: MIT
*/

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "github.com/hortator-ai/Hortator/api/v1alpha1"
)

var (
	spawnPrompt       string
	spawnCapabilities []string
	spawnTimeout      string
	spawnImage        string
	spawnModel        string
	spawnName         string
	spawnRole         string
	spawnTier         string
	spawnParent       string
	spawnWait         bool
	spawnWaitTimeout  string
)

var spawnCmd = &cobra.Command{
	Use:   "spawn",
	Short: "Spawn a new agent task",
	Long: `Spawn a new agent task in the cluster.

Examples:
  hortator spawn --prompt "Write a hello world in Python"
  hortator spawn --prompt "Deploy the app" --capabilities exec,kubernetes
  hortator spawn --prompt "Run tests" --image myregistry/agent:v1 --timeout 1h
  hortator spawn --prompt "Quick task" --wait
  hortator spawn --prompt "Research topic" --role researcher --tier centurion --parent parent-task-123`,
	RunE: runSpawn,
}

func init() {
	spawnCmd.Flags().StringVarP(&spawnPrompt, "prompt", "p", "", "Task prompt (required)")
	spawnCmd.Flags().StringSliceVarP(&spawnCapabilities, "capabilities", "c", nil, "Agent capabilities")
	spawnCmd.Flags().StringVarP(&spawnTimeout, "timeout", "t", "30m", "Task timeout")
	spawnCmd.Flags().StringVarP(&spawnImage, "image", "i", "", "Agent container image")
	spawnCmd.Flags().StringVarP(&spawnModel, "model", "m", "", "LLM model")
	spawnCmd.Flags().StringVar(&spawnName, "name", "", "Task name")
	spawnCmd.Flags().StringVar(&spawnRole, "role", "", "AgentRole or ClusterAgentRole name")
	spawnCmd.Flags().StringVar(&spawnTier, "tier", "", "Hierarchy tier (tribune, centurion, legionary)")
	spawnCmd.Flags().StringVar(&spawnParent, "parent", "", "Parent task name (establishes hierarchy)")
	spawnCmd.Flags().BoolVarP(&spawnWait, "wait", "w", false, "Wait for completion")
	spawnCmd.Flags().StringVar(&spawnWaitTimeout, "wait-timeout", "1h", "Maximum time to wait when --wait is set")
	_ = spawnCmd.MarkFlagRequired("prompt")
	rootCmd.AddCommand(spawnCmd)
}

func runSpawn(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	name := spawnName
	if name == "" {
		name = fmt.Sprintf("task-%d", time.Now().Unix())
	}
	name = strings.ToLower(strings.ReplaceAll(name, " ", "-"))

	// Parse timeout string to seconds
	timeoutDuration, err := time.ParseDuration(spawnTimeout)
	if err != nil {
		return fmt.Errorf("invalid timeout: %w", err)
	}
	timeoutSec := int(timeoutDuration.Seconds())

	task := &corev1alpha1.AgentTask{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: getNamespace(),
		},
		Spec: corev1alpha1.AgentTaskSpec{
			Prompt:       spawnPrompt,
			Capabilities: spawnCapabilities,
			Image:        spawnImage,
			Timeout:      &timeoutSec,
			Role:         spawnRole,
			Tier:         spawnTier,
			ParentTaskID: spawnParent,
		},
	}

	if spawnModel != "" {
		task.Spec.Model = &corev1alpha1.ModelSpec{Name: spawnModel}
	}

	if err := k8sClient.Create(ctx, task); err != nil {
		return fmt.Errorf("failed to create task: %w", err)
	}

	if outputFormat == "json" {
		data, _ := json.MarshalIndent(map[string]string{"name": name, "task": name, "namespace": getNamespace()}, "", "  ")
		fmt.Println(string(data))
		if spawnWait {
			return waitForTask(ctx, name)
		}
		return nil
	}

	fmt.Printf("✓ Task '%s' created in namespace '%s'\n", name, getNamespace())

	if !spawnWait {
		fmt.Printf("\nUse 'hortator status %s' to check progress\n", name)
		return nil
	}

	// Parse wait timeout and create a deadline context
	waitDuration, err := time.ParseDuration(spawnWaitTimeout)
	if err != nil {
		return fmt.Errorf("invalid wait-timeout: %w", err)
	}
	waitCtx, cancel := context.WithTimeout(ctx, waitDuration)
	defer cancel()

	fmt.Println("\nWaiting for task completion...")
	return waitForTask(waitCtx, name)
}

func waitForTask(ctx context.Context, name string) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait timed out (task may still be running)")
		case <-ticker.C:
			task := &corev1alpha1.AgentTask{}
			if err := k8sClient.Get(ctx, client.ObjectKey{
				Namespace: getNamespace(),
				Name:      name,
			}, task); err != nil {
				return fmt.Errorf("failed to get task: %w", err)
			}

			switch task.Status.Phase {
			case corev1alpha1.AgentTaskPhaseCompleted:
				fmt.Printf("✓ Task completed successfully\n")
				if task.Status.Output != "" {
					fmt.Printf("\nOutput:\n%s\n", task.Status.Output)
				}
				return nil
			case corev1alpha1.AgentTaskPhaseFailed:
				return fmt.Errorf("task failed: %s", task.Status.Message)
			case corev1alpha1.AgentTaskPhaseBudgetExceeded:
				fmt.Printf("⚠ Task stopped — budget exceeded\n")
				if task.Status.Output != "" {
					fmt.Printf("\nPartial output:\n%s\n", task.Status.Output)
				}
				return fmt.Errorf("task budget exceeded: %s", task.Status.Message)
			case corev1alpha1.AgentTaskPhaseTimedOut:
				return fmt.Errorf("task timed out: %s", task.Status.Message)
			case corev1alpha1.AgentTaskPhaseCancelled:
				return fmt.Errorf("task was cancelled: %s", task.Status.Message)
			case corev1alpha1.AgentTaskPhaseRunning:
				fmt.Printf("  Running... (pod: %s)\n", task.Status.PodName)
			case corev1alpha1.AgentTaskPhaseRetrying:
				fmt.Printf("  Retrying... (attempt %d)\n", task.Status.Attempts)
			case corev1alpha1.AgentTaskPhasePending:
				fmt.Println("  Pending...")
			}
		}
	}
}
