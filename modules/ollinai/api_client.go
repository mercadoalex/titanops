package ollinai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTPAPIClient is the real HTTP-based implementation of OllinAPIClient.
// It handles authentication, request construction, response parsing,
// and error categorization. This is the infrastructure adapter that the
// Poller engine consumes through the OllinAPIClient interface.
type HTTPAPIClient struct {
	client   *http.Client
	endpoint string
	token    string
}

// HTTPAPIClientConfig configures the HTTPAPIClient.
type HTTPAPIClientConfig struct {
	// Client is the HTTP client to use. If nil, a default client with 30s timeout is created.
	Client *http.Client
	// Endpoint is the OllinAI API base URL (e.g., "https://ollinai.internal:8080").
	Endpoint string
	// AuthToken is the bearer token for authentication.
	AuthToken string
}

// NewHTTPAPIClient creates a new HTTP-based OllinAI API client.
func NewHTTPAPIClient(cfg HTTPAPIClientConfig) *HTTPAPIClient {
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &HTTPAPIClient{
		client:   client,
		endpoint: cfg.Endpoint,
		token:    cfg.AuthToken,
	}
}

// FetchDeploymentRisks retrieves deployment risk entries from the OllinAI REST API.
func (c *HTTPAPIClient) FetchDeploymentRisks(ctx context.Context) ([]DeploymentRiskEntry, error) {
	url := c.endpoint + "/api/v1/deployments/risk"

	body, err := c.doRequest(ctx, url)
	if err != nil {
		return nil, err
	}

	var raw []apiDeploymentRiskResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, &APIError{
			Category: ErrOllinAPIUnavailable,
			Message:  "failed to parse deployment risk response",
			Cause:    err,
		}
	}

	entries := make([]DeploymentRiskEntry, len(raw))
	for i, r := range raw {
		entries[i] = DeploymentRiskEntry(r)
	}

	return entries, nil
}

// FetchDORAMetrics retrieves DORA metrics from the OllinAI REST API.
func (c *HTTPAPIClient) FetchDORAMetrics(ctx context.Context) (*DORAMetrics, error) {
	url := c.endpoint + "/api/v1/metrics/dora"

	body, err := c.doRequest(ctx, url)
	if err != nil {
		return nil, err
	}

	var raw apiDORAMetricsRawResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, &APIError{
			Category: ErrOllinAPIUnavailable,
			Message:  "failed to parse DORA metrics response",
			Cause:    err,
		}
	}

	return &DORAMetrics{
		DeploymentFrequency:  raw.DeploymentFrequency,
		LeadTimeForChanges:   raw.LeadTimeForChanges,
		ChangeFailureRate:    raw.ChangeFailureRate,
		TimeToRestoreService: raw.TimeToRestoreService,
	}, nil
}

// doRequest performs an authenticated GET request and returns the response body.
// On failure, it returns an *APIError with the appropriate category.
func (c *HTTPAPIClient) doRequest(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, &APIError{
			Category: ErrOllinAPIUnavailable,
			Message:  fmt.Sprintf("failed to create request for %s", url),
			Cause:    err,
		}
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, &APIError{
			Category: ErrOllinAPITimeout,
			Message:  fmt.Sprintf("API request failed for %s", url),
			Cause:    err,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, &APIError{
			Category: ErrOllinAPIAuth,
			Message:  fmt.Sprintf("authentication failed for %s: HTTP %d", url, resp.StatusCode),
		}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{
			Category: ErrOllinAPIUnavailable,
			Message:  fmt.Sprintf("API returned non-2xx for %s: HTTP %d", url, resp.StatusCode),
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &APIError{
			Category: ErrOllinAPIUnavailable,
			Message:  fmt.Sprintf("failed to read response body from %s", url),
			Cause:    err,
		}
	}

	return body, nil
}

// --- Wire format structs (JSON deserialization only, internal to this adapter) ---

// apiDeploymentRiskResponse is the JSON wire format for a deployment risk entry.
type apiDeploymentRiskResponse struct {
	Service     string   `json:"service"`
	CommitSHA   string   `json:"commit_sha"`
	Deployer    string   `json:"deployer"`
	RiskScore   int      `json:"risk_score"`
	RiskFactors []string `json:"risk_factors"`
	PipelineID  string   `json:"pipeline_id"`
	Environment string   `json:"environment"`
	Node        string   `json:"node"`
	Pod         string   `json:"pod"`
	Namespace   string   `json:"namespace"`
}

// apiDORAMetricsRawResponse is the JSON wire format for the DORA metrics endpoint.
type apiDORAMetricsRawResponse struct {
	DeploymentFrequency  float64 `json:"deployment_frequency"`
	LeadTimeForChanges   float64 `json:"lead_time_for_changes"`
	ChangeFailureRate    float64 `json:"change_failure_rate"`
	TimeToRestoreService float64 `json:"time_to_restore_service"`
}
