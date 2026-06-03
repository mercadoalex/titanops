package k8s

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// --- ReadSecret Tests ---

func TestReadSecret_Success(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"password": []byte("s3cr3t"),
		},
	}

	clientset := fake.NewSimpleClientset(secret)
	client := NewK8sClient(clientset)

	val, err := client.ReadSecret(context.Background(), "default", "my-secret", "password")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(val) != "s3cr3t" {
		t.Errorf("expected 's3cr3t', got %q", string(val))
	}
}

func TestReadSecret_KeyNotFound(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"username": []byte("admin"),
		},
	}

	clientset := fake.NewSimpleClientset(secret)
	client := NewK8sClient(clientset)

	_, err := client.ReadSecret(context.Background(), "default", "my-secret", "password")
	if err == nil {
		t.Fatal("expected error for missing key")
	}

	var k8sErr *K8sError
	if !errors.As(err, &k8sErr) {
		t.Fatalf("expected *K8sError, got %T", err)
	}
	if k8sErr.Category != ErrNotFound {
		t.Errorf("expected category ErrNotFound, got %s", k8sErr.Category)
	}
}

func TestReadSecret_SecretNotFound(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	client := NewK8sClient(clientset)

	_, err := client.ReadSecret(context.Background(), "default", "nonexistent", "key")
	if err == nil {
		t.Fatal("expected error for missing secret")
	}

	var k8sErr *K8sError
	if !errors.As(err, &k8sErr) {
		t.Fatalf("expected *K8sError, got %T", err)
	}
	if k8sErr.Category != ErrNotFound {
		t.Errorf("expected category ErrNotFound, got %s", k8sErr.Category)
	}
}

// --- ListPods Tests ---

func TestListPods_Success(t *testing.T) {
	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-1",
			Namespace: "default",
			Labels:    map[string]string{"app": "web"},
		},
		Spec: corev1.PodSpec{
			NodeName: "node-1",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}
	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-2",
			Namespace: "default",
			Labels:    map[string]string{"app": "web"},
		},
		Spec: corev1.PodSpec{
			NodeName: "node-2",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	clientset := fake.NewSimpleClientset(pod1, pod2)
	client := NewK8sClient(clientset)

	pods, err := client.ListPods(context.Background(), "default", map[string]string{"app": "web"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pods) != 2 {
		t.Fatalf("expected 2 pods, got %d", len(pods))
	}

	// Verify pod info fields.
	names := map[string]bool{}
	for _, p := range pods {
		names[p.Name] = true
		if p.Namespace != "default" {
			t.Errorf("expected namespace 'default', got %q", p.Namespace)
		}
		if p.Status != "Running" {
			t.Errorf("expected status 'Running', got %q", p.Status)
		}
	}
	if !names["pod-1"] || !names["pod-2"] {
		t.Errorf("expected pods pod-1 and pod-2, got %v", names)
	}
}

func TestListPods_EmptyResult(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	client := NewK8sClient(clientset)

	pods, err := client.ListPods(context.Background(), "default", map[string]string{"app": "nonexistent"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pods) != 0 {
		t.Errorf("expected 0 pods, got %d", len(pods))
	}
}

// --- DeletePod Tests ---

func TestDeletePod_Success(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "target-pod",
			Namespace: "default",
		},
	}

	clientset := fake.NewSimpleClientset(pod)
	client := NewK8sClient(clientset)

	err := client.DeletePod(context.Background(), "default", "target-pod")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify pod is gone.
	_, getErr := clientset.CoreV1().Pods("default").Get(context.Background(), "target-pod", metav1.GetOptions{})
	if !k8serrors.IsNotFound(getErr) {
		t.Errorf("expected pod to be deleted, get returned: %v", getErr)
	}
}

func TestDeletePod_NotFound(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	client := NewK8sClient(clientset)

	err := client.DeletePod(context.Background(), "default", "nonexistent")
	if err == nil {
		t.Fatal("expected error for deleting nonexistent pod")
	}

	var k8sErr *K8sError
	if !errors.As(err, &k8sErr) {
		t.Fatalf("expected *K8sError, got %T", err)
	}
	if k8sErr.Category != ErrNotFound {
		t.Errorf("expected category ErrNotFound, got %s", k8sErr.Category)
	}
}

// --- CordonNode Tests ---

func TestCordonNode_Success(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-1",
		},
		Spec: corev1.NodeSpec{
			Unschedulable: false,
		},
	}

	clientset := fake.NewSimpleClientset(node)
	client := NewK8sClient(clientset)

	err := client.CordonNode(context.Background(), "worker-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify node is cordoned.
	updated, _ := clientset.CoreV1().Nodes().Get(context.Background(), "worker-1", metav1.GetOptions{})
	if !updated.Spec.Unschedulable {
		t.Error("expected node to be unschedulable after cordon")
	}
}

func TestCordonNode_AlreadyCordoned(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-1",
		},
		Spec: corev1.NodeSpec{
			Unschedulable: true, // Already cordoned.
		},
	}

	clientset := fake.NewSimpleClientset(node)
	client := NewK8sClient(clientset)

	err := client.CordonNode(context.Background(), "worker-1")
	if err != nil {
		t.Fatalf("unexpected error for already-cordoned node: %v", err)
	}

	// Verify node remains cordoned.
	updated, _ := clientset.CoreV1().Nodes().Get(context.Background(), "worker-1", metav1.GetOptions{})
	if !updated.Spec.Unschedulable {
		t.Error("expected node to remain unschedulable")
	}
}

// --- RestartPod Tests ---

func TestRestartPod_Success(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "restart-target",
			Namespace: "production",
		},
	}

	clientset := fake.NewSimpleClientset(pod)
	client := NewK8sClient(clientset)

	err := client.RestartPod(context.Background(), "production", "restart-target")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify pod was deleted (restart = delete to trigger controller).
	_, getErr := clientset.CoreV1().Pods("production").Get(context.Background(), "restart-target", metav1.GetOptions{})
	if !k8serrors.IsNotFound(getErr) {
		t.Errorf("expected pod to be deleted for restart, get returned: %v", getErr)
	}
}

// --- Error Classification Tests ---

func TestErrorClassification_NotFound(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	client := NewK8sClient(clientset)

	// Attempt to get a nonexistent secret → NotFound.
	_, err := client.ReadSecret(context.Background(), "ns", "missing", "key")
	if err == nil {
		t.Fatal("expected error")
	}

	var k8sErr *K8sError
	if !errors.As(err, &k8sErr) {
		t.Fatalf("expected *K8sError, got %T", err)
	}
	if k8sErr.Category != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %s", k8sErr.Category)
	}
}

func TestErrorClassification_Forbidden(t *testing.T) {
	clientset := fake.NewSimpleClientset()

	// Inject a Forbidden error for Get secrets.
	clientset.PrependReactor("get", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, k8serrors.NewForbidden(
			schema.GroupResource{Resource: "secrets"},
			"my-secret",
			errors.New("access denied"),
		)
	})

	client := NewK8sClient(clientset)

	_, err := client.ReadSecret(context.Background(), "default", "my-secret", "key")
	if err == nil {
		t.Fatal("expected error")
	}

	var k8sErr *K8sError
	if !errors.As(err, &k8sErr) {
		t.Fatalf("expected *K8sError, got %T", err)
	}
	if k8sErr.Category != ErrPermissionDenied {
		t.Errorf("expected ErrPermissionDenied, got %s", k8sErr.Category)
	}
}
