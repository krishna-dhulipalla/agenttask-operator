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
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	executionv1alpha1 "agenttask.io/operator/api/v1alpha1"
)

var (
	runImage   string
	runTimeout int32
	runTenant  string
	runBackend string
)

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run [file]",
	Short: "Run a python script as an AgentTask",
	Long: `Create an AgentTask from a local python file.
Example: agentctl run my_script.py`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := args[0]
		content, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}

		// Generate a name
		filename := filepath.Base(filePath)
		name := strings.TrimSuffix(filename, filepath.Ext(filename))
		name = strings.ToLower(strings.ReplaceAll(name, "_", "-"))
		// Add some randomness or make it unique? For now, simple.
		// Actually, let's append a timestamp-ish suffix to avoid collision on repeated runs
		// But for CLI simplicity, maybe the user wants to name it?
		// Let's rely on GenerateName metadata feature of K8s, creating with a GenerateName prefix.

		task := &executionv1alpha1.AgentTask{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: name + "-",
				Namespace:    rootNamespace,
			},
			Spec: executionv1alpha1.AgentTaskSpec{
				RuntimeProfile: runImage,
				TimeoutSeconds: runTimeout,
				Tenant:         runTenant,
				Backend:        runBackend,
				Code: executionv1alpha1.CodeSource{
					Source: string(content),
				},
				// Resources not exposed via flag yet for simplicity
			},
		}

		fmt.Printf("Submitting task with code from %s...\n", filePath)
		if err := k8sClient.Create(context.Background(), task); err != nil {
			return err
		}

		fmt.Printf("AgentTask created: %s\n", task.Name)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(runCmd)

	runCmd.Flags().StringVar(&runImage, "profile", "python3.11", "Runtime profile to use")
	runCmd.Flags().Int32Var(&runTimeout, "timeout", 60, "Timeout in seconds")
	runCmd.Flags().StringVar(&runTenant, "tenant", "", "Tenant identifier")
	runCmd.Flags().StringVar(&runBackend, "backend", "", "Execution backend (auto, pod, sandbox)")
}
