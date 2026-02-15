/*
Copyright (c) 2026 hortator-ai
SPDX-License-Identifier: MIT
*/

// Package artifacts provides PVC file extraction via ephemeral pods.
package artifacts

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

// Extractor creates an ephemeral pod that mounts a task's PVC,
// tarballs the /outbox/ contents, and streams them back.
type Extractor struct {
	Clientset  kubernetes.Interface
	RestConfig *rest.Config
}

// ErrPVCNotFound indicates the PVC does not exist.
var ErrPVCNotFound = fmt.Errorf("PVC not found")

// ErrNoFiles indicates no files were found in /outbox/.
var ErrNoFiles = fmt.Errorf("no files found in outbox")

// ListFiles returns a list of file paths in /outbox/ on the PVC.
func (e *Extractor) ListFiles(ctx context.Context, namespace, pvcName string) ([]string, error) {
	if err := e.checkPVC(ctx, namespace, pvcName); err != nil {
		return nil, err
	}

	output, err := e.execOnPVC(ctx, namespace, pvcName, []string{
		"find", "/data/outbox", "-type", "f",
	})
	if err != nil {
		return nil, fmt.Errorf("listing files: %w", err)
	}

	raw := strings.TrimSpace(output)
	if raw == "" {
		return nil, ErrNoFiles
	}

	var files []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			// Return paths relative to /data/outbox/
			files = append(files, strings.TrimPrefix(line, "/data/outbox/"))
		}
	}
	if len(files) == 0 {
		return nil, ErrNoFiles
	}
	return files, nil
}

// DownloadTar streams a tar archive of /outbox/ contents from the PVC.
func (e *Extractor) DownloadTar(ctx context.Context, namespace, pvcName string) (io.ReadCloser, error) {
	if err := e.checkPVC(ctx, namespace, pvcName); err != nil {
		return nil, err
	}

	return e.execStreamOnPVC(ctx, namespace, pvcName, []string{
		"tar", "cf", "-", "-C", "/data/outbox", ".",
	})
}

// DownloadFile streams a single file from the PVC.
func (e *Extractor) DownloadFile(ctx context.Context, namespace, pvcName, filePath string) (io.ReadCloser, error) {
	if err := e.checkPVC(ctx, namespace, pvcName); err != nil {
		return nil, err
	}

	// Sanitize path to prevent directory traversal
	if strings.Contains(filePath, "..") {
		return nil, fmt.Errorf("invalid file path: contains '..'")
	}

	return e.execStreamOnPVC(ctx, namespace, pvcName, []string{
		"cat", fmt.Sprintf("/data/outbox/%s", filePath),
	})
}

func (e *Extractor) checkPVC(ctx context.Context, namespace, pvcName string) error {
	_, err := e.Clientset.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, pvcName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return ErrPVCNotFound
	}
	return err
}

// execOnPVC creates an ephemeral pod, runs a command, and returns stdout as string.
func (e *Extractor) execOnPVC(ctx context.Context, namespace, pvcName string, command []string) (string, error) {
	podName, cleanup, err := e.createHelperPod(ctx, namespace, pvcName)
	if err != nil {
		return "", err
	}
	defer cleanup()

	var stdout, stderr bytes.Buffer
	if err := e.doExec(ctx, namespace, podName, command, &stdout, &stderr); err != nil {
		return "", fmt.Errorf("exec failed: %w (stderr: %s)", err, stderr.String())
	}
	return stdout.String(), nil
}

// execStreamOnPVC creates an ephemeral pod, runs a command, and returns a streaming reader.
// The pod is cleaned up when the returned ReadCloser is closed.
func (e *Extractor) execStreamOnPVC(ctx context.Context, namespace, pvcName string, command []string) (io.ReadCloser, error) {
	podName, cleanup, err := e.createHelperPod(ctx, namespace, pvcName)
	if err != nil {
		return nil, err
	}

	pr, pw := io.Pipe()

	go func() {
		defer cleanup()
		execErr := e.doExec(ctx, namespace, podName, command, pw, io.Discard)
		pw.CloseWithError(execErr)
	}()

	return pr, nil
}

func (e *Extractor) createHelperPod(ctx context.Context, namespace, pvcName string) (string, func(), error) {
	suffix := fmt.Sprintf("%06x", rand.Intn(0xffffff))
	// Derive task name from PVC name (strip -storage suffix)
	taskPart := strings.TrimSuffix(pvcName, "-storage")
	if len(taskPart) > 40 {
		taskPart = taskPart[:40]
	}
	podName := fmt.Sprintf("hortator-artifact-%s-%s", taskPart, suffix)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
			Labels: map[string]string{
				"hortator.ai/artifact-extractor": "true",
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{{
				Name:    "extractor",
				Image:   "busybox:1.37.0",
				Command: []string{"sleep", "300"},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("10m"),
						corev1.ResourceMemory: resource.MustParse("16Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("64Mi"),
					},
				},
				VolumeMounts: []corev1.VolumeMount{{
					Name:      "data",
					MountPath: "/data",
					ReadOnly:  true,
				}},
			}},
			Volumes: []corev1.Volume{{
				Name: "data",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: pvcName,
						ReadOnly:  true,
					},
				},
			}},
		},
	}

	_, err := e.Clientset.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return "", nil, fmt.Errorf("creating extractor pod: %w", err)
	}

	cleanup := func() {
		delCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = e.Clientset.CoreV1().Pods(namespace).Delete(delCtx, podName, metav1.DeleteOptions{})
	}

	// Wait for pod ready
	err = wait.PollUntilContextTimeout(ctx, 500*time.Millisecond, 30*time.Second, true, func(ctx context.Context) (bool, error) {
		p, err := e.Clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		if p.Status.Phase == corev1.PodRunning {
			for _, c := range p.Status.ContainerStatuses {
				if c.Ready {
					return true, nil
				}
			}
		}
		if p.Status.Phase == corev1.PodFailed {
			return false, fmt.Errorf("extractor pod failed")
		}
		return false, nil
	})
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("waiting for extractor pod: %w", err)
	}

	return podName, cleanup, nil
}

func (e *Extractor) doExec(ctx context.Context, namespace, podName string, command []string, stdout io.Writer, stderr io.Writer) error {
	req := e.Clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: "extractor",
			Command:   command,
			Stdout:    true,
			Stderr:    true,
		}, k8sscheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(e.RestConfig, "POST", req.URL())
	if err != nil {
		return fmt.Errorf("creating executor: %w", err)
	}

	return exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: stdout,
		Stderr: stderr,
	})
}
