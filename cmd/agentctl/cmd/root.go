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
	"os"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	executionv1alpha1 "agenttask.io/operator/api/v1alpha1"
)

var (
	scheme       = runtime.NewScheme()
	k8sClient    client.Client
	rootNamespace string
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(executionv1alpha1.AddToScheme(scheme))
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "agentctl",
	Short: "CLI tool for managing AgentTasks",
	Long: `agentctl helps you interact with AgentTasks.
You can run, list, get details, and fetch logs for your tasks directly from the command line.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Initialize Kubernetes Client
		cfg, err := config.GetConfig()
		if err != nil {
			return err
		}

		k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
		if err != nil {
			return err
		}
		return nil
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&rootNamespace, "namespace", "n", "default", "Kubernetes namespace")
}
