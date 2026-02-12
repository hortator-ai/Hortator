/*
Copyright (c) 2026 GeneClackman
SPDX-License-Identifier: MIT
*/

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "github.com/hortator-ai/Hortator/api/v1alpha1"
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
	resultCmd.Flags().BoolVar(&resultArtifacts, "artifacts", false, "Download artifacts from /outbox/artifacts/")
	resultCmd.Flags().StringVar(&resultOutputDir, "output-dir", "./artifacts", "Directory to save downloaded artifacts")
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

// downloadArtifacts reads files from /outbox/artifacts/ on the task's PVC
// by creating a temporary pod and exec-ing into it.
func downloadArtifacts(ctx context.Context, task *corev1alpha1.AgentTask) error {
	if clientset == nil {
		return fmt.Errorf("kubernetes clientset not initialized")
	}

	pvcName := fmt.Sprintf("%s-storage", task.Name)
	ns := getNamespace()

	// Check PVC exists
	_, err := clientset.CoreV1().PersistentVolumeClaims(ns).Get(ctx, pvcName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("PVC %s not found (may have been cleaned up): %w", pvcName, err)
	}

	// Create a temporary pod to read the PVC contents
	readerPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-artifact-reader", task.Name),
			Namespace: ns,
			Labels: map[string]string{
				"hortator.ai/task":   task.Name,
				"hortator.ai/reader": "artifacts",
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:    "reader",
					Image:   "busybox:1.37.0",
					Command: []string{"sleep", "300"},
					VolumeMounts: []corev1.VolumeMount{
						{Name: "storage", MountPath: "/outbox", SubPath: "outbox"},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "storage",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvcName,
							ReadOnly:  true,
						},
					},
				},
			},
		},
	}

	// Create reader pod
	_, err = clientset.CoreV1().Pods(ns).Create(ctx, readerPod, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create artifact reader pod: %w", err)
	}
	defer func() {
		_ = clientset.CoreV1().Pods(ns).Delete(ctx, readerPod.Name, metav1.DeleteOptions{})
	}()

	// Wait for pod to be running
	fmt.Println("Starting artifact reader pod...")
	for i := 0; i < 60; i++ {
		pod, err := clientset.CoreV1().Pods(ns).Get(ctx, readerPod.Name, metav1.GetOptions{})
		if err == nil && pod.Status.Phase == corev1.PodRunning {
			break
		}
		if i == 59 {
			return fmt.Errorf("artifact reader pod did not start in time")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}

	// List artifacts
	fileList, err := execInPod(ctx, ns, readerPod.Name, "reader",
		[]string{"find", "/outbox/artifacts", "-type", "f"})
	if err != nil {
		return fmt.Errorf("failed to list artifacts: %w", err)
	}

	files := strings.Split(strings.TrimSpace(fileList), "\n")
	if len(files) == 0 || (len(files) == 1 && files[0] == "") {
		fmt.Println("No artifacts found")
		return nil
	}

	// Create output directory
	if err := os.MkdirAll(resultOutputDir, 0o755); err != nil {
		return fmt.Errorf("failed to create output dir: %w", err)
	}

	// Download each file
	for _, remotePath := range files {
		remotePath = strings.TrimSpace(remotePath)
		if remotePath == "" {
			continue
		}

		content, err := execInPod(ctx, ns, readerPod.Name, "reader",
			[]string{"cat", remotePath})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to read %s: %v\n", remotePath, err)
			continue
		}

		relPath := strings.TrimPrefix(remotePath, "/outbox/artifacts/")
		localPath := filepath.Join(resultOutputDir, relPath)

		if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to create dir for %s: %v\n", localPath, err)
			continue
		}

		if err := os.WriteFile(localPath, []byte(content), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to write %s: %v\n", localPath, err)
			continue
		}

		fmt.Printf("Downloaded: %s\n", relPath)
	}

	return nil
}

// execInPod runs a command in a pod and returns stdout.
func execInPod(ctx context.Context, ns, podName, container string, command []string) (string, error) {
	config, err := getRESTConfig()
	if err != nil {
		return "", err
	}

	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(ns).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: container,
			Command:   command,
			Stdout:    true,
			Stderr:    true,
		}, k8sscheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return "", fmt.Errorf("failed to create executor: %w", err)
	}

	var stdout, stderr bytes.Buffer
	if err := exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: io.Discard,
	}); err != nil {
		return "", fmt.Errorf("exec failed: %w (stderr: %s)", err, stderr.String())
	}

	return stdout.String(), nil
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
