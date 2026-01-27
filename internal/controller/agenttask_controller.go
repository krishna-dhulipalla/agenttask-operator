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

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

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
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete

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

	// 0. Handle Cancellation
	if agentTask.Spec.Canceled {
		// If already terminal, ignore
		if agentTask.Status.Phase == executionv1alpha1.AgentTaskPhaseSucceeded ||
			agentTask.Status.Phase == executionv1alpha1.AgentTaskPhaseFailed ||
			agentTask.Status.Phase == executionv1alpha1.AgentTaskPhaseCanceled {
			return ctrl.Result{}, nil
		}
		// Otherwise, transition to Canceled and clean up
		return r.reconcileCanceled(ctx, agentTask)
	}

	// State Machine
	switch agentTask.Status.Phase {
	case executionv1alpha1.AgentTaskPhasePending:
		if err := r.reconcilePending(ctx, agentTask); err != nil {
			return ctrl.Result{}, err
		}
		// Requeue to proceed to next state immediately
		return ctrl.Result{Requeue: true}, nil
	case executionv1alpha1.AgentTaskPhaseScheduled:
		if err := r.reconcileScheduled(ctx, agentTask); err != nil {
			return ctrl.Result{}, err
		}
	case executionv1alpha1.AgentTaskPhaseRunning:
		if err := r.reconcileRunning(ctx, agentTask); err != nil {
			return ctrl.Result{}, err
		}
	case executionv1alpha1.AgentTaskPhaseSucceeded, executionv1alpha1.AgentTaskPhaseFailed, executionv1alpha1.AgentTaskPhaseCanceled:
		// Terminal states, no further action needed
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

func (r *AgentTaskReconciler) reconcilePending(ctx context.Context, task *executionv1alpha1.AgentTask) error {
	log := logf.FromContext(ctx)
	log.Info("Reconciling Pending AgentTask", "name", task.Name)

	// 1. Ensure Code ConfigMap
	cmName, err := r.ensureCodeConfigMap(ctx, task)
	if err != nil {
		return err
	}

	// 2. Ensure NetworkPolicy (US-2.3)
	if err := r.ensureNetworkPolicy(ctx, task); err != nil {
		return err
	}

	// 3. Ensure Pod
	if err := r.ensurePod(ctx, task, cmName); err != nil {
		return err
	}

	// 4. Update Status
	task.Status.Phase = executionv1alpha1.AgentTaskPhaseScheduled
	return r.Status().Update(ctx, task)
}

func (r *AgentTaskReconciler) ensureCodeConfigMap(ctx context.Context, task *executionv1alpha1.AgentTask) (string, error) {
	// If configMapRef is provided, verify it exists (optional but good practice)
	if task.Spec.Code.ConfigMapRef != nil {
		return task.Spec.Code.ConfigMapRef.Name, nil
	}

	// If source is provided, create a ConfigMap
	if task.Spec.Code.Source != "" {
		cmName := task.Name + "-code"
		cm := &corev1.ConfigMap{}
		err := r.Get(ctx, types.NamespacedName{Name: cmName, Namespace: task.Namespace}, cm)
		if err != nil && errors.IsNotFound(err) {
			cm = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cmName,
					Namespace: task.Namespace,
					OwnerReferences: []metav1.OwnerReference{
						*metav1.NewControllerRef(task, executionv1alpha1.GroupVersion.WithKind("AgentTask")),
					},
				},
				Data: map[string]string{
					"entrypoint.py": task.Spec.Code.Source, // Assuming Python for now per simple contract
				},
			}
			if err := r.Create(ctx, cm); err != nil {
				return "", err
			}
		} else if err != nil {
			return "", err
		}
		return cmName, nil
	}
	return "", nil
}

func (r *AgentTaskReconciler) ensureNetworkPolicy(ctx context.Context, task *executionv1alpha1.AgentTask) error {
	npName := task.Name + "-netpol"
	np := &networkingv1.NetworkPolicy{}
	err := r.Get(ctx, types.NamespacedName{Name: npName, Namespace: task.Namespace}, np)
	if err != nil && errors.IsNotFound(err) {
		np = &networkingv1.NetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      npName,
				Namespace: task.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(task, executionv1alpha1.GroupVersion.WithKind("AgentTask")),
				},
			},
			Spec: networkingv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"agenttask.io/task": task.Name,
					},
				},
				PolicyTypes: []networkingv1.PolicyType{
					networkingv1.PolicyTypeIngress,
					networkingv1.PolicyTypeEgress,
				},
				Ingress: []networkingv1.NetworkPolicyIngressRule{}, // Deny all ingress
				Egress:  []networkingv1.NetworkPolicyEgressRule{},  // Deny all egress
			},
		}
		if err := r.Create(ctx, np); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	return nil
}

func (r *AgentTaskReconciler) ensurePod(ctx context.Context, task *executionv1alpha1.AgentTask, cmName string) error {
	podName := task.Name + "-pod"
	pod := &corev1.Pod{}
	err := r.Get(ctx, types.NamespacedName{Name: podName, Namespace: task.Namespace}, pod)
	if err != nil && errors.IsNotFound(err) {
		image := r.resolveImage(task.Spec.RuntimeProfile)

		// Prepare Security Contexts
		var runAsNonRoot bool = true
		var allowPrivilegeEscalation bool = false
		var readOnlyRootFilesystem bool = true

		pod = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      podName,
				Namespace: task.Namespace,
				Labels: map[string]string{
					"agenttask.io/task": task.Name,
				},
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(task, executionv1alpha1.GroupVersion.WithKind("AgentTask")),
				},
			},
			Spec: corev1.PodSpec{
				RestartPolicy: corev1.RestartPolicyNever,
				SecurityContext: &corev1.PodSecurityContext{
					RunAsNonRoot: &runAsNonRoot,
					SeccompProfile: &corev1.SeccompProfile{
						Type: corev1.SeccompProfileTypeRuntimeDefault,
					},
				},
				Containers: []corev1.Container{
					{
						Name:    "task",
						Image:   image,
						Command: []string{"python", "/workspace/entrypoint.py"},
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: &allowPrivilegeEscalation,
							ReadOnlyRootFilesystem:   &readOnlyRootFilesystem,
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
							},
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "code",
								MountPath: "/workspace",
								ReadOnly:  true,
							},
							{
								Name:      "tmp",
								MountPath: "/tmp",
							},
						},
						Resources: task.Spec.Resources,
					},
				},
				Volumes: []corev1.Volume{
					{
						Name: "code",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{Name: cmName},
							},
						},
					},
					{
						Name: "tmp",
						VolumeSource: corev1.VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						},
					},
				},
			},
		}

		if err := r.Create(ctx, pod); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	task.Status.PodRef = corev1.ObjectReference{
		Kind:      "Pod",
		Name:      pod.Name,
		Namespace: pod.Namespace,
	}

	return nil
}

func (r *AgentTaskReconciler) reconcileScheduled(ctx context.Context, task *executionv1alpha1.AgentTask) error {
	log := logf.FromContext(ctx)

	// Fetch the Pod
	pod := &corev1.Pod{}
	err := r.Get(ctx, types.NamespacedName{Name: task.Status.PodRef.Name, Namespace: task.Status.PodRef.Namespace}, pod)
	if err != nil {
		if errors.IsNotFound(err) {
			// Pod is missing? Transition to Failed
			log.Error(err, "Pod missing for Scheduled task", "pod", task.Status.PodRef.Name)
			return r.updatePhaseFailure(ctx, task, "PodMissing", "The execution pod was deleted unexpectedly.")
		}
		return err
	}

	// Check Pod Status
	switch pod.Status.Phase {
	case corev1.PodPending:
		// Still waiting
		return nil
	case corev1.PodRunning:
		task.Status.Phase = executionv1alpha1.AgentTaskPhaseRunning
		return r.Status().Update(ctx, task)
	case corev1.PodSucceeded:
		task.Status.Phase = executionv1alpha1.AgentTaskPhaseSucceeded
		now := metav1.Now()
		task.Status.CompletionTime = &now
		return r.Status().Update(ctx, task)
	case corev1.PodFailed:
		return r.handlePodFailure(ctx, task, pod)
	}

	return nil
}

func (r *AgentTaskReconciler) reconcileRunning(ctx context.Context, task *executionv1alpha1.AgentTask) error {
	// Fetch the Pod
	pod := &corev1.Pod{}
	err := r.Get(ctx, types.NamespacedName{Name: task.Status.PodRef.Name, Namespace: task.Status.PodRef.Namespace}, pod)
	if err != nil {
		if errors.IsNotFound(err) {
			return r.updatePhaseFailure(ctx, task, "PodMissing", "The execution pod was deleted while running.")
		}
		return err
	}

	switch pod.Status.Phase {
	case corev1.PodRunning:
		// Still running, check timeout logic here later (US-2.5)
		return nil
	case corev1.PodSucceeded:
		task.Status.Phase = executionv1alpha1.AgentTaskPhaseSucceeded
		now := metav1.Now()
		task.Status.CompletionTime = &now
		return r.Status().Update(ctx, task)
	case corev1.PodFailed:
		return r.handlePodFailure(ctx, task, pod)
	}
	return nil
}

func (r *AgentTaskReconciler) handlePodFailure(ctx context.Context, task *executionv1alpha1.AgentTask, pod *corev1.Pod) error {
	task.Status.Phase = executionv1alpha1.AgentTaskPhaseFailed
	now := metav1.Now()
	task.Status.CompletionTime = &now

	// Extract failure reason
	task.Status.Reason = "PodFailed"
	task.Status.Message = "The execution pod failed."

	// Try to get exit code
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name == "task" && status.State.Terminated != nil {
			task.Status.ExitCode = status.State.Terminated.ExitCode
			if status.State.Terminated.Message != "" {
				task.Status.Message = status.State.Terminated.Message
			}
			task.Status.Reason = status.State.Terminated.Reason
			break
		}
	}

	return r.Status().Update(ctx, task)
}

func (r *AgentTaskReconciler) updatePhaseFailure(ctx context.Context, task *executionv1alpha1.AgentTask, reason, message string) error {
	task.Status.Phase = executionv1alpha1.AgentTaskPhaseFailed
	task.Status.Reason = reason
	task.Status.Message = message
	now := metav1.Now()
	task.Status.CompletionTime = &now
	return r.Status().Update(ctx, task)
}

func (r *AgentTaskReconciler) reconcileCanceled(ctx context.Context, task *executionv1alpha1.AgentTask) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("Canceling AgentTask", "name", task.Name)

	// Attempt to delete the Pod if it exists
	if task.Status.PodRef.Name != "" {
		pod := &corev1.Pod{}
		err := r.Get(ctx, types.NamespacedName{Name: task.Status.PodRef.Name, Namespace: task.Status.PodRef.Namespace}, pod)
		if err == nil {
			// Pod exists, delete it
			if err := r.Delete(ctx, pod); err != nil {
				if !errors.IsNotFound(err) {
					log.Error(err, "Failed to delete pod for canceled task")
					return ctrl.Result{}, err
				}
			}
		} else if !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}

	// Update Status
	task.Status.Phase = executionv1alpha1.AgentTaskPhaseCanceled
	now := metav1.Now()
	task.Status.CompletionTime = &now
	task.Status.Message = "Task was canceled by user request"
	task.Status.Reason = "Canceled"

	if err := r.Status().Update(ctx, task); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *AgentTaskReconciler) resolveImage(profile string) string {
	// Simple mapping for MVP
	switch profile {
	case "python3.11", "python3.10":
		return "python:3.11-slim"
	default:
		return "python:3.11-slim"
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *AgentTaskReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&executionv1alpha1.AgentTask{}).
		Named("agenttask").
		Complete(r)
}
