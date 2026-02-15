/*
Copyright (c) 2026 hortator-ai
SPDX-License-Identifier: MIT
*/

package cmd

import (
	"archive/tar"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "github.com/hortator-ai/Hortator/api/v1alpha1"
	"github.com/hortator-ai/Hortator/internal/artifacts"
)

var (
	resultWait      bool
	resultArtifacts bool
	resultOutputDir string
)

var resultCmd = &cobra.Command{
	Use:   "result <task-name>",
	Short: "Get the result of a completed task",
	Long: `Get the result/output of a completed agent task.

Examples:
  hortator result my-task
  hortator result my-task --json
  hortator result my-task --wait
  hortator result my-task --artifacts --output-dir ./output`,
	Args: cobra.ExactArgs(1),
	RunE: runResult,
}

func init() {
	resultCmd.Flags().BoolVarP(&resultWait, "wait", "w", false, "Wait for completion")
	resultCmd.Flags().BoolVar(&resultArtifacts, "artifacts", false, "Download artifacts from /outbox/")
	resultCmd.Flags().StringVar(&resultOutputDir, "output-dir", ".", "Directory to save downloaded artifacts")
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
	case corev1alpha1.AgentTaskPhaseCompleted, corev1alpha1.AgentTaskPhaseFailed,
		corev1alpha1.AgentTaskPhaseBudgetExceeded, corev1alpha1.AgentTaskPhaseTimedOut,
		corev1alpha1.AgentTaskPhaseCancelled:
		// Terminal phases — ok to read result
	case corev1alpha1.AgentTaskPhasePending:
		return fmt.Errorf("task is still pending")
	case corev1alpha1.AgentTaskPhaseRunning:
		return fmt.Errorf("task is still running (use --wait)")
	case corev1alpha1.AgentTaskPhaseWaiting:
		return fmt.Errorf("task is waiting for children to complete")
	default:
		return fmt.Errorf("unknown task phase: %s", task.Status.Phase)
	}

	// Download artifacts if requested
	if resultArtifacts {
		if err := downloadArtifacts(ctx, task); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: artifact download failed: %v\n", err)
		}
	}

	if outputFormat == "json" {
		result := map[string]interface{}{
			"name":      task.Name,
			"namespace": task.Namespace,
			"phase":     task.Status.Phase,
			"message":   task.Status.Message,
			"output":    task.Status.Output,
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

// downloadArtifacts reads files from /outbox/ on the task's PVC using the shared Extractor.
func downloadArtifacts(ctx context.Context, task *corev1alpha1.AgentTask) error {
	ext, err := newExtractor()
	if err != nil {
		return err
	}

	pvcName := fmt.Sprintf("%s-storage", task.Name)
	ns := getNamespace()

	fmt.Println("Starting artifact extraction...")

	rc, err := ext.DownloadTar(ctx, ns, pvcName)
	if err != nil {
		if errors.Is(err, artifacts.ErrPVCNotFound) {
			return fmt.Errorf("PVC %s not found (may have been cleaned up)", pvcName)
		}
		return err
	}
	defer func() { _ = rc.Close() }()

	if err := os.MkdirAll(resultOutputDir, 0o755); err != nil {
		return fmt.Errorf("creating output dir: %w", err)
	}

	return untarTo(rc, resultOutputDir)
}

// untarTo extracts a tar stream to the given directory and prints each file.
func untarTo(r io.Reader, dir string) error {
	tr := tar.NewReader(r)
	count := 0
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			if count > 0 {
				// Partial extraction is OK — tar stream may end abruptly
				return nil
			}
			return fmt.Errorf("reading tar: %w", err)
		}

		target := filepath.Join(dir, hdr.Name)

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				_ = f.Close()
				return err
			}
			_ = f.Close()
			fmt.Printf("  %s\n", hdr.Name)
			count++
		}
	}
	if count == 0 {
		fmt.Println("No artifacts found in outbox")
	} else {
		fmt.Printf("Extracted %d file(s) to %s\n", count, dir)
	}
	return nil
}

// newExtractor creates an Extractor using the CLI's k8s config.
func newExtractor() (*artifacts.Extractor, error) {
	if clientset == nil {
		return nil, fmt.Errorf("kubernetes clientset not initialized")
	}
	cfg, err := getRESTConfig()
	if err != nil {
		return nil, err
	}
	return &artifacts.Extractor{
		Clientset:  clientset,
		RestConfig: cfg,
	}, nil
}

// getRESTConfig returns the kubernetes rest config.
func getRESTConfig() (*rest.Config, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		loadingRules.ExplicitPath = kubeconfig
	}
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	return kubeConfig.ClientConfig()
}
