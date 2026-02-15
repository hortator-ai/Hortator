/*
Copyright (c) 2026 hortator-ai
SPDX-License-Identifier: MIT
*/

package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/hortator-ai/Hortator/internal/artifacts"
)

var artifactsOutputDir string

var artifactsCmd = &cobra.Command{
	Use:   "artifacts",
	Short: "Manage task artifacts from PVC outbox",
	Long: `List, download, and retrieve individual artifact files from a task's PVC.

Examples:
  hortator artifacts list my-task
  hortator artifacts download my-task --output-dir ./out
  hortator artifacts get my-task results/report.txt`,
}

var artifactsListCmd = &cobra.Command{
	Use:   "list <task-name>",
	Short: "List artifact files in the task's outbox",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		ext, err := newExtractor()
		if err != nil {
			return err
		}
		pvcName := fmt.Sprintf("%s-storage", args[0])
		files, err := ext.ListFiles(ctx, getNamespace(), pvcName)
		if err != nil {
			if errors.Is(err, artifacts.ErrPVCNotFound) {
				return fmt.Errorf("PVC %s not found (may have been cleaned up)", pvcName)
			}
			if errors.Is(err, artifacts.ErrNoFiles) {
				fmt.Println("No artifacts found")
				return nil
			}
			return err
		}
		for _, f := range files {
			fmt.Println(f)
		}
		return nil
	},
}

var artifactsDownloadCmd = &cobra.Command{
	Use:   "download <task-name>",
	Short: "Download all artifacts from the task's outbox",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		ext, err := newExtractor()
		if err != nil {
			return err
		}
		pvcName := fmt.Sprintf("%s-storage", args[0])
		rc, err := ext.DownloadTar(ctx, getNamespace(), pvcName)
		if err != nil {
			if errors.Is(err, artifacts.ErrPVCNotFound) {
				return fmt.Errorf("PVC %s not found (may have been cleaned up)", pvcName)
			}
			return err
		}
		defer rc.Close()

		if err := os.MkdirAll(artifactsOutputDir, 0o755); err != nil {
			return err
		}
		return untarTo(rc, artifactsOutputDir)
	},
}

var artifactsGetCmd = &cobra.Command{
	Use:   "get <task-name> <path>",
	Short: "Download a single artifact file to stdout",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		ext, err := newExtractor()
		if err != nil {
			return err
		}
		pvcName := fmt.Sprintf("%s-storage", args[0])
		rc, err := ext.DownloadFile(ctx, getNamespace(), pvcName, args[1])
		if err != nil {
			if errors.Is(err, artifacts.ErrPVCNotFound) {
				return fmt.Errorf("PVC %s not found (may have been cleaned up)", pvcName)
			}
			return err
		}
		defer rc.Close()
		_, err = io.Copy(os.Stdout, rc)
		return err
	},
}

func init() {
	artifactsDownloadCmd.Flags().StringVar(&artifactsOutputDir, "output-dir", ".", "Directory to save downloaded artifacts")
	artifactsCmd.AddCommand(artifactsListCmd)
	artifactsCmd.AddCommand(artifactsDownloadCmd)
	artifactsCmd.AddCommand(artifactsGetCmd)
	rootCmd.AddCommand(artifactsCmd)
}
