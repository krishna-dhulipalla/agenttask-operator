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

	"github.com/spf13/cobra"

	executionv1alpha1 "agenttask.io/operator/api/v1alpha1"
)

// deleteCmd represents the delete command
var deleteCmd = &cobra.Command{
	Use:   "delete [name]",
	Short: "Delete an AgentTask",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		task := &executionv1alpha1.AgentTask{}
		// We need to fetch it first to get ResourceVersion for deletion?
		// Actually client.Delete only needs ObjectMeta with Name/Namespace for basic delete
		task.Name = name
		task.Namespace = rootNamespace

		err := k8sClient.Delete(context.Background(), task)
		if err != nil {
			return err
		}

		fmt.Printf("AgentTask %s deleted\n", name)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(deleteCmd)
}
