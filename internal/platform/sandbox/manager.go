package sandbox

import (
	"context"
	"fmt"
	
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	executionv1alpha1 "agenttask.io/operator/api/v1alpha1"
)

var (
	SandboxGVK = schema.GroupVersionKind{
		Group:   "execution.agenttask.io",
		Version: "v1alpha1",
		Kind:    "Sandbox",
	}
)

type Manager struct {
	Client client.Client
}

func (m *Manager) EnsureSandbox(ctx context.Context, task *executionv1alpha1.AgentTask, cmName string, image string) (*corev1.ObjectReference, error) {
	sandboxName := task.Name + "-sandbox"
	
	// Check if exists
	sandbox := &unstructured.Unstructured{}
	sandbox.SetGroupVersionKind(SandboxGVK)
	err := m.Client.Get(ctx, client.ObjectKey{Name: sandboxName, Namespace: task.Namespace}, sandbox)
	if err == nil {
		// Already exists
		return &corev1.ObjectReference{
			Kind:      SandboxGVK.Kind,
			Name:      sandbox.GetName(),
			Namespace: sandbox.GetNamespace(),
		}, nil
	}
	if client.IgnoreNotFound(err) != nil {
		return nil, err
	}

	// Create
	sandbox = &unstructured.Unstructured{}
	sandbox.SetGroupVersionKind(SandboxGVK)
	sandbox.SetName(sandboxName)
	sandbox.SetNamespace(task.Namespace)
	
	// Construct Spec
	// This mirrors the structure of the Mock CRD we created
	sandbox.Object["spec"] = map[string]interface{}{
		"image": image,
		"command": []string{"python", "/workspace/entrypoint.py"},
		// We would map volumes/mounts here if the Sandbox API supports it
		// For MVP mock, we just set image/command
	}

	// Set Owner Ref
	sandbox.SetOwnerReferences([]metav1.OwnerReference{
		*metav1.NewControllerRef(task, executionv1alpha1.GroupVersion.WithKind("AgentTask")),
	})

	if err := m.Client.Create(ctx, sandbox); err != nil {
		return nil, fmt.Errorf("failed to create sandbox: %w", err)
	}

	return &corev1.ObjectReference{
		Kind:      SandboxGVK.Kind,
		Name:      sandbox.GetName(),
		Namespace: sandbox.GetNamespace(),
	}, nil
}


