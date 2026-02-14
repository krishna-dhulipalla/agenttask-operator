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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/duration"

	executionv1alpha1 "agenttask.io/operator/api/v1alpha1"
)

// describeCmd represents the describe command
var describeCmd = &cobra.Command{
	Use:   "describe [name]",
	Short: "Show details of a specific AgentTask",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		task := &executionv1alpha1.AgentTask{}
		err := k8sClient.Get(context.Background(), types.NamespacedName{Name: name, Namespace: rootNamespace}, task)
		if err != nil {
			return err
		}

	w := tabwriter.NewWriter(os.Stdout, 0, 8, 1, '\t', 0)

	_, _ = fmt.Fprintf(w, "Name:\t%s\n", task.Name)
	_, _ = fmt.Fprintf(w, "Namespace:\t%s\n", task.Namespace)
	_, _ = fmt.Fprintf(w, "Labels:\t%v\n", task.Labels)
	_, _ = fmt.Fprintf(w, "Annotations:\t%v\n", task.Annotations)

	_, _ = fmt.Fprintln(w, "\nStatus:")
	if task.Status.Reason != "" {
		_, _ = fmt.Fprintf(w, "  Reason:\t%s\n", task.Status.Reason)
	}
	if task.Status.Message != "" {
		_, _ = fmt.Fprintf(w, "  Message:\t%s\n", task.Status.Message)
	}

	if task.Status.StartTime != nil {
		_, _ = fmt.Fprintf(w, "  Start Time:\t%s\n", task.Status.StartTime.Time.Format(time.RFC3339))
	}
	if task.Status.CompletionTime != nil {
		_, _ = fmt.Fprintf(w, "  Completion Time:\t%s\n", task.Status.CompletionTime.Time.Format(time.RFC3339))
		if task.Status.StartTime != nil {
			d := task.Status.CompletionTime.Time.Sub(task.Status.StartTime.Time)
			_, _ = fmt.Fprintf(w, "  Duration:\t%s\n", duration.HumanDuration(d))
		}
	}

	_, _ = fmt.Fprintln(w, "\nExecution:")
	if task.Status.PodRef.Name != "" {
		_, _ = fmt.Fprintf(w, "  Pod Name:\t%s\n", task.Status.PodRef.Name)
	}
	_, _ = fmt.Fprintf(w, "  Exit Code:\t%d\n", task.Status.ExitCode)

	if task.Status.Result != nil {
		_, _ = fmt.Fprintln(w, "\nResult:")
		if task.Status.Result.JSON != "" {
			_, _ = fmt.Fprintf(w, "  JSON:\t%s\n", task.Status.Result.JSON)
		}
	}

	return w.Flush()
		return nil
	},
}

func init() {
	rootCmd.AddCommand(describeCmd)
}
