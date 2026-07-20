package ebeecontrol

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Deployer defines the interface for honeytoken deployment into Kubernetes pods.
// Implementations manage the full lifecycle: deploy, undeploy, status queries.
type Deployer interface {
	// Deploy creates honeytokens in the target pod. Returns deployment results.
	// On partial failure, already-deployed honeytokens from the batch are rolled back.
	Deploy(ctx context.Context, request DeploymentRequest) (DeploymentResponse, error)

	// Undeploy removes a honeytoken by ID.
	Undeploy(ctx context.Context, honeytokenID string) error

	// GetDeploymentStatus returns the current status of a deployed honeytoken.
	GetDeploymentStatus(ctx context.Context, honeytokenID string) (DeploymentStatus, error)
}

// K8sSecretClient abstracts Kubernetes Secret/ConfigMap operations for DI.
// In production, this wraps client-go's CoreV1 API.
type K8sSecretClient interface {
	// CreateSecret creates a namespaced Secret with the given name, namespace, labels, and data.
	CreateSecret(ctx context.Context, namespace, name string, labels map[string]string, data map[string][]byte) error
	// DeleteSecret deletes a namespaced Secret by name.
	DeleteSecret(ctx context.Context, namespace, name string) error
}

// K8sDeployer implements Deployer using the Kubernetes API to create
// Secrets and ConfigMaps as honeytoken artifacts.
type K8sDeployer struct {
	client K8sSecretClient
	mu     sync.RWMutex
	store  map[string]DeployedHoneytoken
}

// NewK8sDeployer creates a deployer backed by the Kubernetes API.
func NewK8sDeployer(client K8sSecretClient) *K8sDeployer {
	return &K8sDeployer{
		client: client,
		store:  make(map[string]DeployedHoneytoken),
	}
}

// Deploy creates honeytoken secrets in the target pod's namespace.
// Validates the request, creates K8s Secrets, and on any failure rolls back
// all previously deployed honeytokens from the same batch.
func (d *K8sDeployer) Deploy(ctx context.Context, req DeploymentRequest) (DeploymentResponse, error) {
	if err := validateDeploymentRequest(req); err != "" {
		return DeploymentResponse{
			Success:             false,
			DeployedHoneytokens: nil,
			Errors: []DeploymentError{{
				PodID:              req.PodID,
				FailureReason:      err,
				RemediationActions: []string{"retry_deployment"},
			}},
		}, nil
	}

	var deployed []DeployedHoneytoken

	for _, spec := range req.Honeytokens {
		honeytokenID := uuid.New().String()
		secretName := fmt.Sprintf("ebeecontrol-ht-%s", honeytokenID[:8])
		content := generateDecoyContent(spec, honeytokenID)

		labels := map[string]string{
			"app.kubernetes.io/managed-by": "ebeecontrol",
			"ebeecontrol.io/honeytoken-id": honeytokenID,
			"ebeecontrol.io/type":          string(spec.Type),
		}

		data := map[string][]byte{
			spec.Name: content,
		}

		if err := d.client.CreateSecret(ctx, req.Namespace, secretName, labels, data); err != nil {
			// Roll back all previously deployed honeytokens from this batch.
			for _, ht := range deployed {
				rollbackName := fmt.Sprintf("ebeecontrol-ht-%s", ht.HoneytokenID[:8])
				_ = d.client.DeleteSecret(ctx, ht.Namespace, rollbackName)
				d.mu.Lock()
				delete(d.store, ht.HoneytokenID)
				d.mu.Unlock()
			}

			return DeploymentResponse{
				Success:             false,
				DeployedHoneytokens: nil,
				Errors: []DeploymentError{{
					PodID:              req.PodID,
					FailureReason:      err.Error(),
					RemediationActions: selectRemediationActions(err.Error()),
				}},
			}, nil
		}

		ht := DeployedHoneytoken{
			HoneytokenID:        honeytokenID,
			PodID:               req.PodID,
			Namespace:           req.Namespace,
			Type:                spec.Type,
			FilePath:            spec.Placement,
			DeploymentTimestamp: time.Now().UTC(),
		}

		d.mu.Lock()
		d.store[honeytokenID] = ht
		d.mu.Unlock()

		deployed = append(deployed, ht)
	}

	return DeploymentResponse{
		Success:             true,
		DeployedHoneytokens: deployed,
		Errors:              nil,
	}, nil
}

// Undeploy removes a honeytoken Secret from Kubernetes.
func (d *K8sDeployer) Undeploy(ctx context.Context, honeytokenID string) error {
	d.mu.RLock()
	ht, exists := d.store[honeytokenID]
	d.mu.RUnlock()

	if !exists {
		return nil
	}

	secretName := fmt.Sprintf("ebeecontrol-ht-%s", honeytokenID[:8])
	_ = d.client.DeleteSecret(ctx, ht.Namespace, secretName)

	d.mu.Lock()
	delete(d.store, honeytokenID)
	d.mu.Unlock()

	return nil
}

// GetDeploymentStatus returns the lifecycle status of a deployed honeytoken.
func (d *K8sDeployer) GetDeploymentStatus(_ context.Context, honeytokenID string) (DeploymentStatus, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if _, exists := d.store[honeytokenID]; exists {
		return DeploymentActive, nil
	}
	return DeploymentNotFound, nil
}

// --- In-Memory Deployer (for testing / simulation) ---

// InMemoryDeployer implements Deployer without Kubernetes, storing honeytokens in memory.
type InMemoryDeployer struct {
	mu    sync.RWMutex
	store map[string]DeployedHoneytoken
}

// NewInMemoryDeployer creates a deployer that stores honeytokens in memory.
func NewInMemoryDeployer() *InMemoryDeployer {
	return &InMemoryDeployer{
		store: make(map[string]DeployedHoneytoken),
	}
}

// Deploy simulates honeytoken deployment in memory.
func (d *InMemoryDeployer) Deploy(_ context.Context, req DeploymentRequest) (DeploymentResponse, error) {
	if err := validateDeploymentRequest(req); err != "" {
		return DeploymentResponse{
			Success:             false,
			DeployedHoneytokens: nil,
			Errors: []DeploymentError{{
				PodID:              req.PodID,
				FailureReason:      err,
				RemediationActions: []string{"retry_deployment"},
			}},
		}, nil
	}

	var deployed []DeployedHoneytoken

	for _, spec := range req.Honeytokens {
		honeytokenID := uuid.New().String()
		ht := DeployedHoneytoken{
			HoneytokenID:        honeytokenID,
			PodID:               req.PodID,
			Namespace:           req.Namespace,
			Type:                spec.Type,
			FilePath:            spec.Placement,
			DeploymentTimestamp: time.Now().UTC(),
		}

		d.mu.Lock()
		d.store[honeytokenID] = ht
		d.mu.Unlock()

		deployed = append(deployed, ht)
	}

	return DeploymentResponse{
		Success:             true,
		DeployedHoneytokens: deployed,
		Errors:              nil,
	}, nil
}

// Undeploy removes a honeytoken from memory.
func (d *InMemoryDeployer) Undeploy(_ context.Context, honeytokenID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.store, honeytokenID)
	return nil
}

// GetDeploymentStatus returns the status of an in-memory honeytoken.
func (d *InMemoryDeployer) GetDeploymentStatus(_ context.Context, honeytokenID string) (DeploymentStatus, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if _, exists := d.store[honeytokenID]; exists {
		return DeploymentActive, nil
	}
	return DeploymentNotFound, nil
}

// --- Helpers ---

// validateDeploymentRequest checks a deployment request for validity.
// Returns an empty string if valid, or an error message if invalid.
func validateDeploymentRequest(req DeploymentRequest) string {
	if strings.TrimSpace(req.PodID) == "" {
		return "podId is required"
	}
	if strings.TrimSpace(req.Namespace) == "" {
		return "namespace is required"
	}
	if len(req.Honeytokens) < 1 {
		return "at least 1 honeytoken must be specified"
	}
	if len(req.Honeytokens) > 5 {
		return "at most 5 honeytokens can be deployed per pod"
	}
	return ""
}

// selectRemediationActions chooses appropriate remediation actions based on the failure reason.
func selectRemediationActions(reason string) []string {
	r := strings.ToLower(reason)

	switch {
	case strings.Contains(r, "permission") || strings.Contains(r, "forbidden") || strings.Contains(r, "access denied"):
		return []string{"select_alternative_pod", "escalate_to_operator"}
	case strings.Contains(r, "timeout") || strings.Contains(r, "network") || strings.Contains(r, "connection"):
		return []string{"retry_deployment", "select_alternative_pod"}
	case strings.Contains(r, "resource") || strings.Contains(r, "capacity") || strings.Contains(r, "quota"):
		return []string{"select_alternative_pod", "escalate_to_operator"}
	default:
		return []string{"retry_deployment", "escalate_to_operator"}
	}
}

// generateDecoyContent produces realistic decoy data based on honeytoken type.
func generateDecoyContent(spec HoneytokenSpec, honeytokenID string) []byte {
	if spec.Content != "" {
		return []byte(spec.Content)
	}

	switch spec.Type {
	case HoneytokenDecoySecret:
		return []byte(fmt.Sprintf(
			`{"apiVersion":"v1","kind":"ServiceAccountToken","token":"eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.%s.DECOY_DO_NOT_USE","expirationTimestamp":"2030-01-01T00:00:00Z"}`,
			honeytokenID,
		))
	case HoneytokenDecoyCredential:
		return []byte(fmt.Sprintf(
			"-----BEGIN RSA PRIVATE KEY-----\nDECOY-%s-DO-NOT-USE\n-----END RSA PRIVATE KEY-----\n",
			honeytokenID,
		))
	case HoneytokenDecoyFile:
		return []byte(fmt.Sprintf(
			`{"aws_access_key_id":"AKIA%s","aws_secret_access_key":"DECOY/%s/DO_NOT_USE","region":"us-east-1","_warning":"THIS IS A HONEYTOKEN - ACCESS WILL BE DETECTED"}`,
			strings.ToUpper(strings.ReplaceAll(honeytokenID, "-", ""))[:16],
			honeytokenID,
		))
	default:
		return []byte(fmt.Sprintf("DECOY-%s", honeytokenID))
	}
}
