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
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/duration"
	"sigs.k8s.io/controller-runtime/pkg/client"

	executionv1alpha1 "agenttask.io/operator/api/v1alpha1"
)

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all AgentTasks",
	RunE: func(cmd *cobra.Command, args []string) error {
		taskList := &executionv1alpha1.AgentTaskList{}
		err := k8sClient.List(context.Background(), taskList, client.InNamespace(rootNamespace))
		if err != nil {
			return err
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "NAME\tPHASE\tAGE\tPOD")

		for _, task := range taskList.Items {
			age := "Unknown"
			if task.CreationTimestamp.Time.IsZero() == false {
				age = duration.HumanDuration(time.Since(task.CreationTimestamp.Time))
			}
			Phase := string(task.Status.Phase)
			if Phase == "" {
				Phase = "Pending"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", task.Name, Phase, age, task.Status.PodRef.Name)
		}
		w.Flush()
		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
