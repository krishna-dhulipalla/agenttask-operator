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

package controller

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	executionv1alpha1 "agenttask.io/operator/api/v1alpha1"
)

// AgentTaskReconciler reconciles a AgentTask object
type AgentTaskReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=execution.agenttask.io,resources=agenttasks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=execution.agenttask.io,resources=agenttasks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=execution.agenttask.io,resources=agenttasks/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the AgentTask object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.0/pkg/reconcile
func (r *AgentTaskReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = logf.FromContext(ctx)

	// Fetch the AgentTask instance
	agentTask := &executionv1alpha1.AgentTask{}
	if err := r.Get(ctx, req.NamespacedName, agentTask); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Initialize Phase if empty
	if agentTask.Status.Phase == "" {
		agentTask.Status.Phase = executionv1alpha1.AgentTaskPhasePending
		now := metav1.Now()
		agentTask.Status.StartTime = &now
		if err := r.Status().Update(ctx, agentTask); err != nil {
			return ctrl.Result{}, err
		}
		// Return to trigger immediate re-reconcile
		return ctrl.Result{Requeue: true}, nil
	}

	// State Machine
	switch agentTask.Status.Phase {
	case executionv1alpha1.AgentTaskPhasePending:
		// TODO: Implement scheduling logic (US-2.1)
		// For now, we transition to Scheduled to simulate progress if US-2.1 is next.
		// In a real scenario, we would wait for the Pod creation.
		logf.Log.Info("AgentTask is Pending", "name", agentTask.Name)
	case executionv1alpha1.AgentTaskPhaseScheduled:
		// TODO: Watch Pod logic
	case executionv1alpha1.AgentTaskPhaseRunning:
		// TODO: Monitor Pod logic
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AgentTaskReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&executionv1alpha1.AgentTask{}).
		Named("agenttask").
		Complete(r)
}
