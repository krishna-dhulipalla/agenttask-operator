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

package v1alpha1

import (
	"context"
	"fmt"
	"slices"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	executionv1alpha1 "agenttask.io/operator/api/v1alpha1"
)

// nolint:unused
// log is for logging in this package.
var agenttasklog = logf.Log.WithName("agenttask-resource")

// SetupAgentTaskWebhookWithManager registers the webhook for AgentTask in the manager.
func SetupAgentTaskWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &executionv1alpha1.AgentTask{}).
		WithValidator(&AgentTaskCustomValidator{Client: mgr.GetClient()}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: If you want to customise the 'path', use the flags '--defaulting-path' or '--validation-path'.
// +kubebuilder:webhook:path=/validate-execution-agenttask-io-v1alpha1-agenttask,mutating=false,failurePolicy=fail,sideEffects=None,groups=execution.agenttask.io,resources=agenttasks,verbs=create;update,versions=v1alpha1,name=vagenttask-v1alpha1.kb.io,admissionReviewVersions=v1

// AgentTaskCustomValidator struct is responsible for validating the AgentTask resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type AgentTaskCustomValidator struct {
	Client client.Client
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type AgentTask.
func (v *AgentTaskCustomValidator) ValidateCreate(ctx context.Context, obj *executionv1alpha1.AgentTask) (admission.Warnings, error) {
	agenttasklog.Info("Validation for AgentTask upon creation", "name", obj.GetName())

	// 1. Static Validation
	if err := validateAgentTask(obj); err != nil {
		return nil, err
	}

	// 2. Concurrency Limit Check (US-6.2)
	// Ignore if coming from DryRun
	// We'll list all tasks in the same namespace
	taskList := &executionv1alpha1.AgentTaskList{}
	if err := v.Client.List(ctx, taskList, client.InNamespace(obj.Namespace)); err != nil {
		return nil, fmt.Errorf("failed to list tasks for concurrency check: %w", err)
	}

	activeCount := 0
	const MaxActiveTasks = 5

	for _, task := range taskList.Items {
		// Count tasks that are NOT in a terminal state
		if task.Status.Phase != executionv1alpha1.AgentTaskPhaseSucceeded &&
			task.Status.Phase != executionv1alpha1.AgentTaskPhaseFailed &&
			task.Status.Phase != executionv1alpha1.AgentTaskPhaseTimeout &&
			task.Status.Phase != executionv1alpha1.AgentTaskPhaseCanceled {
			activeCount++
		}
	}

	if activeCount >= MaxActiveTasks {
		return nil, fmt.Errorf("too many active tasks in namespace %s (max %d)", obj.Namespace, MaxActiveTasks)
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type AgentTask.
func (v *AgentTaskCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj *executionv1alpha1.AgentTask) (admission.Warnings, error) {
	agenttasklog.Info("Validation for AgentTask upon update", "name", newObj.GetName())

	// Immutability Check
	if newObj.Spec.RuntimeProfile != oldObj.Spec.RuntimeProfile {
		return nil, fmt.Errorf("runtimeProfile is immutable")
	}
	if newObj.Spec.Code.Source != oldObj.Spec.Code.Source {
		return nil, fmt.Errorf("code.source is immutable")
	}

	return nil, validateAgentTask(newObj)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type AgentTask.
func (v *AgentTaskCustomValidator) ValidateDelete(ctx context.Context, obj *executionv1alpha1.AgentTask) (admission.Warnings, error) {
	return nil, nil
}

func validateAgentTask(task *executionv1alpha1.AgentTask) error {
	// 1. Runtime Profile
	allowedProfiles := []string{"python3.10", "python3.11"}
	if !slices.Contains(allowedProfiles, task.Spec.RuntimeProfile) {
		return fmt.Errorf("invalid runtimeProfile '%s'. Allowed: %v", task.Spec.RuntimeProfile, allowedProfiles)
	}

	// 2. Timeouts
	if task.Spec.TimeoutSeconds <= 0 {
		return fmt.Errorf("timeoutSeconds must be greater than 0")
	}
	if task.Spec.TimeoutSeconds > 3600 {
		return fmt.Errorf("timeoutSeconds cannot exceed 3600 (1 hour)")
	}

	// 3. Resources (Simulate a cap)
	// For example, max 4 CPU, 8Gi Memory.
	// This would require parsing Quantity, skipping for MVP simplicity but structure is here.

	return nil
}
