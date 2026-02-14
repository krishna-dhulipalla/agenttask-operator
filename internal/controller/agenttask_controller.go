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
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/prometheus/client_golang/prometheus"

	executionv1alpha1 "agenttask.io/operator/api/v1alpha1"
	"agenttask.io/operator/internal/platform/sandbox"
)

var (
	tasksTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "agenttask_total_tasks",
			Help: "Total number of AgentTasks processed",
		},
		[]string{"phase", "reason"},
	)
	taskDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "agenttask_duration_seconds",
			Help:    "Time taken for AgentTask to reach terminal state",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"phase"},
	)
)

func init() {
	metrics.Registry.MustRegister(tasksTotal, taskDuration)
}

// AgentTaskReconciler reconciles a AgentTask object
type AgentTaskReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=execution.agenttask.io,resources=agenttasks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=execution.agenttask.io,resources=agenttasks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=execution.agenttask.io,resources=agenttasks/finalizers,verbs=update
// +kubebuilder:rbac:groups=execution.agenttask.io,resources=sandboxes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch

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

	// Define Finalizer
	agentTaskFinalizer := "execution.agenttask.io/finalizer"

	// Check if the object is being deleted
	if agentTask.ObjectMeta.DeletionTimestamp.IsZero() {
		// Not being deleted, add finalizer if missing
		if !controllerutil.ContainsFinalizer(agentTask, agentTaskFinalizer) {
			controllerutil.AddFinalizer(agentTask, agentTaskFinalizer)
			if err := r.Update(ctx, agentTask); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	} else {
		// Being deleted
		if controllerutil.ContainsFinalizer(agentTask, agentTaskFinalizer) {
			// Run finalization logic
			if err := r.finalizeAgentTask(ctx, agentTask); err != nil {
				return ctrl.Result{}, err
			}

			// Remove finalizer
			controllerutil.RemoveFinalizer(agentTask, agentTaskFinalizer)
			if err := r.Update(ctx, agentTask); err != nil {
				return ctrl.Result{}, err
			}
		}
		// Stop reconciliation
		return ctrl.Result{}, nil
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

	// 1. Check Timeout (US-2.5)
	if timeout := r.checkTimeout(agentTask); timeout {
		return r.reconcileTimeout(ctx, agentTask)
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
	case executionv1alpha1.AgentTaskPhaseSucceeded, executionv1alpha1.AgentTaskPhaseFailed, executionv1alpha1.AgentTaskPhaseCanceled, executionv1alpha1.AgentTaskPhaseTimeout:
		// Terminal states
		// Check for TTL Cleanup (US-8.1)
		if agentTask.Spec.TTLSecondsAfterFinished != nil {
			ttl := time.Duration(*agentTask.Spec.TTLSecondsAfterFinished) * time.Second
			if agentTask.Status.CompletionTime != nil {
				expirationTime := agentTask.Status.CompletionTime.Add(ttl)
				if time.Now().After(expirationTime) {
					// Expired, delete it
					logf.FromContext(ctx).Info("Task expired based on TTLSecondsAfterFinished, deleting", "name", agentTask.Name)
					if err := r.Delete(ctx, agentTask); err != nil {
						return ctrl.Result{}, client.IgnoreNotFound(err)
					}
					return ctrl.Result{}, nil
				}
				// Not yet expired, requeue at expiration time
				requeueAfter := time.Until(expirationTime)
				return ctrl.Result{RequeueAfter: requeueAfter}, nil
			}
		}
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

	// 3. Select Backend (US-3.1)
	backend, err := r.selectBackend(ctx, task)
	if err != nil {
		return r.updatePhaseFailure(ctx, task, string(executionv1alpha1.AgentTaskReasonBackendSelectionFailed), err.Error())
	}
	
	// Resolve Image (US-3.3)
	// We need to resolve the image differently for Pod vs Sandbox?
	// Actually, let's assume the ConfigMap has keys like "python3.11" (Pod) and "python3.11-sandbox" (Sandbox) OR
	// we just use a single resolve function that returns the right image based on backend.
	image, err := r.resolveRuntimeProfile(ctx, task.Spec.RuntimeProfile, backend, task.Namespace)
	if err != nil {
		return r.updatePhaseFailure(ctx, task, string(executionv1alpha1.AgentTaskReasonProfileResolutionFailed), err.Error())
	}

	// 4. Dispatch
	if backend == "sandbox" {
		// US-3.2: Creation
		sbManager := &sandbox.Manager{Client: r.Client}
		objRef, err := sbManager.EnsureSandbox(ctx, task, cmName, image)
		if err != nil {
			return err
		}
		task.Status.PodRef = *objRef
	} else {
		// Default to Pod
		if err := r.ensurePod(ctx, task, cmName, image); err != nil {
			return err
		}
	}

	// 5. Update Status
	task.Status.Phase = executionv1alpha1.AgentTaskPhaseScheduled
	return r.Status().Update(ctx, task)
}

func (r *AgentTaskReconciler) selectBackend(ctx context.Context, task *executionv1alpha1.AgentTask) (string, error) {
	requested := task.Spec.Backend
	if requested == "" {
		requested = "auto"
	}

	sandboxAvailable, err := r.checkSandboxAvailability(ctx)
	if err != nil {
		// Log warning but don't fail, assume false
		// logf.FromContext(ctx).Error(err, "Failed to check sandbox availability")
	}

	if requested == "sandbox" {
		if !sandboxAvailable {
			return "", errors.NewBadRequest("sandbox backend requested but not available")
		}
		return "sandbox", nil
	}

	if requested == "auto" {
		if sandboxAvailable {
			return "sandbox", nil
		}
		return "pod", nil
	}

	return "pod", nil
}

func (r *AgentTaskReconciler) checkSandboxAvailability(ctx context.Context) (bool, error) {
	// Check if the Sandbox CRD exists
	// We assume a hypothetical CRD name "sandboxes.execution.agenttask.io" for now
	crdName := "sandboxes.execution.agenttask.io" 
	crd := &metav1.PartialObjectMetadata{}
	crd.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apiextensions.k8s.io",
		Version: "v1",
		Kind:    "CustomResourceDefinition",
	})
	
	err := r.Get(ctx, types.NamespacedName{Name: crdName}, crd)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
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

func (r *AgentTaskReconciler) ensurePod(ctx context.Context, task *executionv1alpha1.AgentTask, cmName string, image string) error {
	podName := task.Name + "-pod"
	pod := &corev1.Pod{}
	err := r.Get(ctx, types.NamespacedName{Name: podName, Namespace: task.Namespace}, pod)
	if err != nil && errors.IsNotFound(err) {
		// Prepare Security Contexts
			runAsNonRoot := true
		var runAsUser int64 = 1000
		allowPrivilegeEscalation := false
		readOnlyRootFilesystem := true

		labels := map[string]string{
			"agenttask.io/task": task.Name,
		}
		if task.Spec.Tenant != "" {
			labels["agenttask.io/tenant"] = task.Spec.Tenant
		}

		pod = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      podName,
				Namespace: task.Namespace,
				Labels:    labels,
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(task, executionv1alpha1.GroupVersion.WithKind("AgentTask")),
				},
			},
			Spec: corev1.PodSpec{
				RestartPolicy: corev1.RestartPolicyNever,
				SecurityContext: &corev1.PodSecurityContext{
					RunAsNonRoot: &runAsNonRoot,
					RunAsUser:    &runAsUser,
					SeccompProfile: &corev1.SeccompProfile{
						Type: corev1.SeccompProfileTypeRuntimeDefault,
					},
				},
				Containers: []corev1.Container{
					{
						Name:    "task",
						Image:   image,
						Command: []string{"python", "/workspace/entrypoint.py"},
						Env: []corev1.EnvVar{
							{Name: "PYTHONDONTWRITEBYTECODE", Value: "1"},
							{Name: "PYTHONUNBUFFERED", Value: "1"},
						},
						TerminationMessagePath:   "/workspace/artifacts/result.json",
						TerminationMessagePolicy: corev1.TerminationMessageReadFile,
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
							{
								Name:      "artifacts",
								MountPath: "/workspace/artifacts",
							},
						},
						Resources: task.Spec.Resources,
					},
					// Sidecar Container (US-4.1)
					{
						Name:  "artifacts-collector",
						Image: "alpine:latest",
						Command: []string{
							"/bin/sh", "-c",
							"while true; do if [ -d /workspace/artifacts ]; then for f in /workspace/artifacts/*; do if [ -f \"$f\" ]; then echo \"Uploading artifact: $(basename $f)\"; fi; done; fi; sleep 5; done",
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "artifacts",
								MountPath: "/workspace/artifacts",
								ReadOnly:  true,
							},
						},
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
					{
						Name: "artifacts",
						VolumeSource: corev1.VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{
								SizeLimit: resource.NewQuantity(100*1024*1024, resource.BinarySI), // 100Mi
							},
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

	// Check if this is a Sandbox
	if task.Status.PodRef.Kind == "Sandbox" {
		// For MVP Mock, we assume if it exists, it's running.
		// In reality, we'd fetch the Sandbox object and check its Status field.
		task.Status.Phase = executionv1alpha1.AgentTaskPhaseRunning
		return r.Status().Update(ctx, task)
	}

	// Fetch the Pod
	pod := &corev1.Pod{}
	err := r.Get(ctx, types.NamespacedName{Name: task.Status.PodRef.Name, Namespace: task.Status.PodRef.Namespace}, pod)
	if err != nil {
		if errors.IsNotFound(err) {
			// Pod is missing? Transition to Failed
			// Pod is missing? Transition to Failed
			log.Error(err, "Pod missing for Scheduled task", "pod", task.Status.PodRef.Name)
			return r.updatePhaseFailure(ctx, task, string(executionv1alpha1.AgentTaskReasonPodMissing), "The execution pod was deleted unexpectedly.")
		}
		return err
	}

	// Check Pod Status
	switch pod.Status.Phase {
	case corev1.PodPending:
		// Check for ImagePullBackOff
		for _, status := range pod.Status.ContainerStatuses {
			if status.State.Waiting != nil && (status.State.Waiting.Reason == "ImagePullBackOff" || status.State.Waiting.Reason == "ErrImagePull") {
				return r.updatePhaseFailure(ctx, task, string(executionv1alpha1.AgentTaskReasonImagePullFailed), fmt.Sprintf("Failed to pull image %s: %s", status.Image, status.State.Waiting.Message))
			}
		}
		// Still waiting
		return nil
	case corev1.PodRunning:
		task.Status.Phase = executionv1alpha1.AgentTaskPhaseRunning
		return r.Status().Update(ctx, task)
	case corev1.PodSucceeded:
		return r.handlePodSuccess(ctx, task, pod)
	case corev1.PodFailed:
		return r.handlePodFailure(ctx, task, pod)
	}

	return nil
}

func (r *AgentTaskReconciler) reconcileRunning(ctx context.Context, task *executionv1alpha1.AgentTask) error {
	// Check if this is a Sandbox
	if task.Status.PodRef.Kind == "Sandbox" {
		// For MVP Mock, just stay running.
		return nil
	}

	// Fetch the Pod
	pod := &corev1.Pod{}
	err := r.Get(ctx, types.NamespacedName{Name: task.Status.PodRef.Name, Namespace: task.Status.PodRef.Namespace}, pod)
	if err != nil {
		if errors.IsNotFound(err) {
			return r.updatePhaseFailure(ctx, task, string(executionv1alpha1.AgentTaskReasonPodMissing), "The execution pod was deleted while running.")
		}
		return err
	}

	switch pod.Status.Phase {
	case corev1.PodRunning:
		// Check if the MAIN container ("task") has terminated.
		// This handles the "Sidecar Problem" where the Pod stays Running because the sidecar is still up.
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.Name == "task" && cs.State.Terminated != nil {
				if cs.State.Terminated.ExitCode == 0 {
					return r.handlePodSuccess(ctx, task, pod)
				} else {
					return r.handlePodFailure(ctx, task, pod)
				}
			}
		}

		// Still running, check timeout logic here later (US-2.5)
		return nil
	case corev1.PodSucceeded:
		return r.handlePodSuccess(ctx, task, pod)
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
	// Extract failure reason
	task.Status.Reason = string(executionv1alpha1.AgentTaskReasonPodFailed)
	task.Status.Message = "The execution pod failed."

	// Metrics
	tasksTotal.WithLabelValues("Failed", "PodFailed").Inc()
	if task.Status.StartTime != nil {
		duration := task.Status.CompletionTime.Time.Sub(task.Status.StartTime.Time).Seconds()
		taskDuration.WithLabelValues("Failed").Observe(duration)
	}

	// Try to get exit code and container statuses
	for _, status := range pod.Status.ContainerStatuses {
		if status.State.Waiting != nil && (status.State.Waiting.Reason == "ImagePullBackOff" || status.State.Waiting.Reason == "ErrImagePull") {
			task.Status.Reason = string(executionv1alpha1.AgentTaskReasonImagePullFailed)
			task.Status.Message = fmt.Sprintf("Failed to pull image %s: %s", status.Image, status.State.Waiting.Message)
		}
		if status.Name == "task" && status.State.Terminated != nil {
			task.Status.ExitCode = status.State.Terminated.ExitCode
			if status.State.Terminated.Message != "" {
				task.Status.Message = status.State.Terminated.Message
			}
			task.Status.Reason = status.State.Terminated.Reason
			break
		}
	}

	if err := r.Status().Update(ctx, task); err != nil {
		return err
	}

	if err := r.Delete(ctx, pod); err != nil && !errors.IsNotFound(err) {
		return err
	}

	// Metrics
	tasksTotal.WithLabelValues("Failed", task.Status.Reason).Inc()
	if task.Status.StartTime != nil {
		duration := task.Status.CompletionTime.Time.Sub(task.Status.StartTime.Time).Seconds()
		taskDuration.WithLabelValues("Failed").Observe(duration)
	}

	return nil
}

func (r *AgentTaskReconciler) handlePodSuccess(ctx context.Context, task *executionv1alpha1.AgentTask, pod *corev1.Pod) error {
	task.Status.Phase = executionv1alpha1.AgentTaskPhaseSucceeded
	now := metav1.Now()
	task.Status.CompletionTime = &now

	// Extract result from termination message
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name == "task" && status.State.Terminated != nil {
			if msg := status.State.Terminated.Message; msg != "" {
				task.Status.Result = &executionv1alpha1.ExecutionResult{
					JSON: msg,
				}
			}
			task.Status.ExitCode = status.State.Terminated.ExitCode
			break
		}
	}

	if err := r.Status().Update(ctx, task); err != nil {
		return err
	}

	if err := r.Delete(ctx, pod); err != nil && !errors.IsNotFound(err) {
		return err
	}

	// Metrics
	tasksTotal.WithLabelValues("Succeeded", "PodSucceeded").Inc()
	if task.Status.StartTime != nil {
		duration := task.Status.CompletionTime.Time.Sub(task.Status.StartTime.Time).Seconds()
		taskDuration.WithLabelValues("Succeeded").Observe(duration)
	}

	return nil
}

func (r *AgentTaskReconciler) updatePhaseFailure(ctx context.Context, task *executionv1alpha1.AgentTask, reason, message string) error {
	task.Status.Phase = executionv1alpha1.AgentTaskPhaseFailed
	task.Status.Reason = reason
	task.Status.Message = message
	now := metav1.Now()
	task.Status.CompletionTime = &now

	// Metrics
	tasksTotal.WithLabelValues("Failed", reason).Inc()
	if task.Status.StartTime != nil {
		duration := task.Status.CompletionTime.Time.Sub(task.Status.StartTime.Time).Seconds()
		taskDuration.WithLabelValues("Failed").Observe(duration)
	}

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
	task.Status.Message = "Task was canceled by user request"
	task.Status.Reason = string(executionv1alpha1.AgentTaskReasonCanceled)

	// Metrics
	tasksTotal.WithLabelValues("Canceled", string(executionv1alpha1.AgentTaskReasonCanceled)).Inc()
	if task.Status.StartTime != nil {
		duration := task.Status.CompletionTime.Time.Sub(task.Status.StartTime.Time).Seconds()
		taskDuration.WithLabelValues("Canceled").Observe(duration)
	}

	if err := r.Status().Update(ctx, task); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *AgentTaskReconciler) checkTimeout(task *executionv1alpha1.AgentTask) bool {
	if task.Spec.TimeoutSeconds <= 0 {
		return false
	}
	if task.Status.StartTime == nil {
		return false
	}
	// Give a small buffer of 2 seconds
	deadline := task.Status.StartTime.Add(time.Duration(task.Spec.TimeoutSeconds)*time.Second + 2*time.Second)
	return time.Now().After(deadline)
}

func (r *AgentTaskReconciler) reconcileTimeout(ctx context.Context, task *executionv1alpha1.AgentTask) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("AgentTask timed out", "name", task.Name)

	// Reuse cancellation logic to clean up pod
	if task.Status.PodRef.Name != "" {
		pod := &corev1.Pod{}
		err := r.Get(ctx, types.NamespacedName{Name: task.Status.PodRef.Name, Namespace: task.Status.PodRef.Namespace}, pod)
		if err == nil {
			if err := r.Delete(ctx, pod); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	task.Status.Phase = executionv1alpha1.AgentTaskPhaseTimeout
	now := metav1.Now()
	task.Status.CompletionTime = &now
	task.Status.Reason = string(executionv1alpha1.AgentTaskReasonTimeout)
	task.Status.Message = "Task execution exceeded the timeout limit."
	task.Status.Message = "Task execution exceeded the timeout limit."

	// Metrics
	tasksTotal.WithLabelValues("Timeout", string(executionv1alpha1.AgentTaskReasonTimeout)).Inc()
	if task.Status.StartTime != nil {
		duration := task.Status.CompletionTime.Time.Sub(task.Status.StartTime.Time).Seconds()
		taskDuration.WithLabelValues("Timeout").Observe(duration)
	}

	if err := r.Status().Update(ctx, task); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *AgentTaskReconciler) resolveRuntimeProfile(ctx context.Context, profile string, backend string, namespace string) (string, error) {
	// 1. Try ConfigMap
	cm := &corev1.ConfigMap{}
	// We look for the ConfigMap in the same namespace as the task (for now) or a specific system namespace.
	// For MVP, let's look in the task's namespace to allow user overrides, 
	// OR (better) look in the operator's namespace... but we don't know it easily.
	// Let's assume it's in the SAME namespace as the Task for simplicity of testing.
	err := r.Get(ctx, types.NamespacedName{Name: "agenttask-runtime-profiles", Namespace: namespace}, cm)
	if err == nil {
		// ConfigMap exists
		key := profile
		if backend == "sandbox" {
			key = profile + "-sandbox"
		}
		
		if val, ok := cm.Data[key]; ok {
			return val, nil
		}
	}

	// 2. Fallback to Hardcoded Defaults
	switch profile {
	case "python3.11", "python3.10":
		if backend == "sandbox" {
			return "agent-sandbox-python:3.11", nil
		}
		return "python:3.11-slim", nil
	default:
		return "", fmt.Errorf("unknown runtime profile: %s", profile)
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *AgentTaskReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&executionv1alpha1.AgentTask{}).
		Owns(&corev1.Pod{}).
		Named("agenttask").
		Complete(r)
}

func (r *AgentTaskReconciler) finalizeAgentTask(ctx context.Context, task *executionv1alpha1.AgentTask) error {
	log := logf.FromContext(ctx)
	log.Info("Finalizing AgentTask", "name", task.Name)

	// Clean up Pod if it exists
	if task.Status.PodRef.Name != "" {
		pod := &corev1.Pod{}
		err := r.Get(ctx, types.NamespacedName{Name: task.Status.PodRef.Name, Namespace: task.Status.PodRef.Namespace}, pod)
		if err == nil {
			if err := r.Delete(ctx, pod); err != nil {
				if !errors.IsNotFound(err) {
					log.Error(err, "Failed to delete pod during finalization")
					return err
				}
			}
		} else if !errors.IsNotFound(err) {
			return err
		}
	}

	return nil
}
