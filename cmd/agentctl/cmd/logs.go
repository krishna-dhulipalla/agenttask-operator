/*
Copyright 2026 AgentTask Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	executionv1alpha1 "agenttask.io/operator/api/v1alpha1"
)

// logsCmd represents the logs command
var logsCmd = &cobra.Command{
	Use:   "logs [name]",
	Short: "Get logs from the AgentTask execution",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		ctx := context.Background()

		// 1. Get Task
		task := &executionv1alpha1.AgentTask{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: rootNamespace}, task)
		if err != nil {
			return fmt.Errorf("failed to get task: %w", err)
		}

		if task.Status.PodRef.Name == "" {
			return fmt.Errorf("task %s has no pod assigned yet (Phase: %s)", name, task.Status.Phase)
		}

		// 2. Create CoreV1 Client for logs
		cfg, err := config.GetConfig()
		if err != nil {
			return err
		}
		clientset, err := kubernetes.NewForConfig(cfg)
		if err != nil {
			return err
		}

		// 3. Stream Logs
		req := clientset.CoreV1().Pods(task.Status.PodRef.Namespace).GetLogs(task.Status.PodRef.Name, &corev1.PodLogOptions{
			Container: "task",
			Follow:    false, // TODO: Add -f flag support
		})

		podLogs, err := req.Stream(ctx)
		if err != nil {
			return fmt.Errorf("error in opening stream: %w", err)
		}
		defer podLogs.Close()

		_, err = io.Copy(os.Stdout, podLogs)
		if err != nil {
			return fmt.Errorf("error in copy information from podLogs to stdout: %w", err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(logsCmd)
}
