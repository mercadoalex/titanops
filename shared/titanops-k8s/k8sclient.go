package k8s

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

// ErrorCategory classifies the type of Kubernetes API failure.
type ErrorCategory string

const (
	// ErrAPIUnreachable indicates the Kubernetes API server could not be reached.
	ErrAPIUnreachable ErrorCategory = "api_unreachable"
	// ErrNotFound indicates the requested resource does not exist.
	ErrNotFound ErrorCategory = "not_found"
	// ErrPermissionDenied indicates the caller lacks permission for the operation.
	ErrPermissionDenied ErrorCategory = "permission_denied"
	// ErrUnknown indicates an unclassified failure.
	ErrUnknown ErrorCategory = "unknown"
)

// K8sError is a typed error returned by all Client operations.
// It classifies the failure category and includes the underlying cause.
type K8sError struct {
	Category ErrorCategory
	Resource string
	Message  string
	Cause    error
}

// Error implements the error interface.
func (e *K8sError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("k8s %s error on %s: %s: %v", e.Category, e.Resource, e.Message, e.Cause)
	}
	return fmt.Sprintf("k8s %s error on %s: %s", e.Category, e.Resource, e.Message)
}

// Unwrap returns the underlying cause for use with errors.Is/As.
func (e *K8sError) Unwrap() error {
	return e.Cause
}

// classifyError converts a Kubernetes API error into the appropriate ErrorCategory.
func classifyError(err error, resource string, message string) *K8sError {
	if err == nil {
		return nil
	}

	var category ErrorCategory
	switch {
	case k8serrors.IsNotFound(err):
		category = ErrNotFound
	case k8serrors.IsForbidden(err), k8serrors.IsUnauthorized(err):
		category = ErrPermissionDenied
	case k8serrors.IsServerTimeout(err), k8serrors.IsServiceUnavailable(err), k8serrors.IsTimeout(err):
		category = ErrAPIUnreachable
	default:
		// Check if the error is a connection-level failure (API unreachable).
		if isConnectionError(err) {
			category = ErrAPIUnreachable
		} else {
			category = ErrUnknown
		}
	}

	return &K8sError{
		Category: category,
		Resource: resource,
		Message:  message,
		Cause:    err,
	}
}

// isConnectionError checks if an error indicates a network-level connection failure.
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	// Connection refused, DNS resolution failures, and similar network errors
	// typically appear as generic errors without a StatusError wrapper.
	if _, ok := err.(*k8serrors.StatusError); !ok {
		errMsg := err.Error()
		// Common connection failure patterns
		for _, pattern := range []string{
			"connection refused",
			"no such host",
			"i/o timeout",
			"network is unreachable",
			"dial tcp",
		} {
			if contains(errMsg, pattern) {
				return true
			}
		}
	}
	return false
}

// contains checks if s contains substr (case-insensitive would be better,
// but for error classification simple substring matching is sufficient).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// K8sClient implements the Client interface using client-go.
type K8sClient struct {
	clientset kubernetes.Interface
}

// NewK8sClient creates a new K8sClient wrapping the given clientset.
// The clientset is accepted as an interface for testability with fake clients.
func NewK8sClient(clientset kubernetes.Interface) *K8sClient {
	return &K8sClient{clientset: clientset}
}

// ReadSecret retrieves a secret value by namespace, name, and key.
func (c *K8sClient) ReadSecret(ctx context.Context, namespace, name, key string) ([]byte, error) {
	secret, err := c.clientset.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, classifyError(err, fmt.Sprintf("secret/%s/%s", namespace, name), "failed to get secret")
	}

	value, ok := secret.Data[key]
	if !ok {
		return nil, &K8sError{
			Category: ErrNotFound,
			Resource: fmt.Sprintf("secret/%s/%s[%s]", namespace, name, key),
			Message:  fmt.Sprintf("key %q not found in secret", key),
		}
	}

	return value, nil
}

// ListPods returns pods matching the given label selector in a namespace.
func (c *K8sClient) ListPods(ctx context.Context, namespace string, selector map[string]string) ([]PodInfo, error) {
	labelSelector := labels.SelectorFromSet(labels.Set(selector)).String()

	podList, err := c.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, classifyError(err, fmt.Sprintf("pods/%s", namespace), "failed to list pods")
	}

	pods := make([]PodInfo, 0, len(podList.Items))
	for _, pod := range podList.Items {
		pods = append(pods, podToPodInfo(&pod))
	}

	return pods, nil
}

// DeletePod removes a pod by namespace and name.
func (c *K8sClient) DeletePod(ctx context.Context, namespace, name string) error {
	err := c.clientset.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return classifyError(err, fmt.Sprintf("pod/%s/%s", namespace, name), "failed to delete pod")
	}
	return nil
}

// CordonNode marks a node as unschedulable.
func (c *K8sClient) CordonNode(ctx context.Context, nodeName string) error {
	node, err := c.clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return classifyError(err, fmt.Sprintf("node/%s", nodeName), "failed to get node")
	}

	if node.Spec.Unschedulable {
		// Already cordoned, nothing to do.
		return nil
	}

	node.Spec.Unschedulable = true
	_, err = c.clientset.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
	if err != nil {
		return classifyError(err, fmt.Sprintf("node/%s", nodeName), "failed to cordon node")
	}

	return nil
}

// RestartPod deletes a pod to trigger restart by its controller.
func (c *K8sClient) RestartPod(ctx context.Context, namespace, name string) error {
	err := c.clientset.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return classifyError(err, fmt.Sprintf("pod/%s/%s", namespace, name), "failed to restart pod")
	}
	return nil
}

// podToPodInfo converts a corev1.Pod to a PodInfo struct.
func podToPodInfo(pod *corev1.Pod) PodInfo {
	return PodInfo{
		Name:      pod.Name,
		Namespace: pod.Namespace,
		NodeName:  pod.Spec.NodeName,
		Status:    string(pod.Status.Phase),
		Labels:    pod.Labels,
	}
}
