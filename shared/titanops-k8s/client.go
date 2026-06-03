// Package k8s provides common Kubernetes client operations
// used across TitanOps modules.
package k8s

import "context"

// Client provides common Kubernetes operations used across modules.
// All methods accept context.Context as the first parameter and return
// typed errors without panicking on failure.
type Client interface {
	// ReadSecret retrieves a secret value by namespace, name, and key.
	// Returns the raw secret bytes or an error if the secret is not found
	// or the API is unreachable.
	ReadSecret(ctx context.Context, namespace, name, key string) ([]byte, error)

	// ListPods returns pods matching the given label selector in a namespace.
	// The selector map is applied as equality-based label requirements.
	ListPods(ctx context.Context, namespace string, selector map[string]string) ([]PodInfo, error)

	// DeletePod removes a pod by namespace and name.
	// Returns an error if the pod does not exist or cannot be deleted.
	DeletePod(ctx context.Context, namespace, name string) error

	// CordonNode marks a node as unschedulable, preventing new pods
	// from being scheduled onto it.
	CordonNode(ctx context.Context, nodeName string) error

	// RestartPod deletes a pod to trigger restart by its controller.
	// This is equivalent to DeletePod but communicates intent for restart.
	RestartPod(ctx context.Context, namespace, name string) error
}

// PodInfo contains basic pod information returned by ListPods.
type PodInfo struct {
	// Name is the pod's name.
	Name string
	// Namespace is the pod's namespace.
	Namespace string
	// NodeName is the node where the pod is running.
	NodeName string
	// Status is the pod's current phase (e.g., Running, Pending, Failed).
	Status string
	// Labels contains the pod's labels.
	Labels map[string]string
}
