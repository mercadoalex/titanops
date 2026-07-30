package ebeecontrol

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DynatraceClient provides service discovery, pod context queries,
// access event subscription, and forensic report submission
// via the Dynatrace MCP Server API.
type DynatraceClient struct {
	endpointURL      string
	discoveryTimeout time.Duration
	maxRetries       int
	contextTimeout   time.Duration
	httpClient       HTTPClient
	callbacks        []func(AccessEvent)
}

// HTTPClient abstracts HTTP operations for dependency injection.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// DynatraceClientConfig configures the Dynatrace MCP Server client.
type DynatraceClientConfig struct {
	// EndpointURL is the base URL of the Dynatrace MCP Server.
	EndpointURL string
	// DiscoveryTimeout is the timeout for service discovery queries. Default 30s.
	DiscoveryTimeout time.Duration
	// MaxRetries is the max retry count for discovery queries. Default 5.
	MaxRetries int
	// ContextTimeout is the timeout for pod context queries. Default 3s.
	ContextTimeout time.Duration
}

// NewDynatraceClient creates a client for the Dynatrace MCP Server.
// If httpClient is nil, http.DefaultClient is used.
func NewDynatraceClient(cfg DynatraceClientConfig, httpClient HTTPClient) *DynatraceClient {
	if cfg.DiscoveryTimeout == 0 {
		cfg.DiscoveryTimeout = 30 * time.Second
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 5
	}
	if cfg.ContextTimeout == 0 {
		cfg.ContextTimeout = 3 * time.Second
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &DynatraceClient{
		endpointURL:      cfg.EndpointURL,
		discoveryTimeout: cfg.DiscoveryTimeout,
		maxRetries:       cfg.MaxRetries,
		contextTimeout:   cfg.ContextTimeout,
		httpClient:       httpClient,
	}
}

// QueryHighRiskServices queries the Dynatrace MCP Server for high-risk services.
// Applies a timeout per attempt and retries with exponential backoff (starting at 2s).
// Returns an empty slice when no services are found (not an error).
func (c *DynatraceClient) QueryHighRiskServices(ctx context.Context) ([]HighRiskService, error) {
	var lastErr error
	backoff := 2 * time.Second

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > 32*time.Second {
				backoff = 32 * time.Second
			}
		}

		services, err := c.fetchHighRiskServices(ctx)
		if err != nil {
			lastErr = err
			continue
		}
		return services, nil
	}

	return nil, fmt.Errorf("query high-risk services failed after %d retries: %w", c.maxRetries, lastErr)
}

// fetchHighRiskServices performs a single attempt to fetch high-risk services.
func (c *DynatraceClient) fetchHighRiskServices(ctx context.Context) ([]HighRiskService, error) {
	reqCtx, cancel := context.WithTimeout(ctx, c.discoveryTimeout)
	defer cancel()

	url := fmt.Sprintf("%s/api/v1/services/high-risk", c.endpointURL)
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Dynatrace MCP Server returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var result struct {
		Services []HighRiskService `json:"services"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if result.Services == nil {
		return []HighRiskService{}, nil
	}
	return result.Services, nil
}

// GetPodContext queries the Dynatrace MCP Server for pod context information.
// Applies a short timeout (default 3s) and returns nil on any failure
// (caller defaults to high-risk classification).
func (c *DynatraceClient) GetPodContext(ctx context.Context, podID, namespace string) *PodContext {
	reqCtx, cancel := context.WithTimeout(ctx, c.contextTimeout)
	defer cancel()

	url := fmt.Sprintf("%s/api/v1/pods/%s/context?namespace=%s", c.endpointURL, podID, namespace)
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	var podCtx PodContext
	if err := json.Unmarshal(body, &podCtx); err != nil {
		return nil
	}
	return &podCtx
}

// OnAccessEvent registers a callback to be invoked when access events arrive.
func (c *DynatraceClient) OnAccessEvent(callback func(AccessEvent)) {
	c.callbacks = append(c.callbacks, callback)
}

// EmitAccessEvent dispatches an access event to all registered callbacks.
// Used for testing/simulation or when events arrive from Tetragon via Dynatrace.
func (c *DynatraceClient) EmitAccessEvent(event AccessEvent) {
	for _, cb := range c.callbacks {
		cb(event)
	}
}

// SubmitForensicReport posts a forensic report to the Dynatrace MCP Server.
// Retries with exponential backoff on failure.
func (c *DynatraceClient) SubmitForensicReport(ctx context.Context, report ForensicReport) error {
	var lastErr error
	backoff := 2 * time.Second

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > 32*time.Second {
				backoff = 32 * time.Second
			}
		}

		if err := c.postForensicReport(ctx, report); err != nil {
			lastErr = err
			continue
		}
		return nil
	}

	return fmt.Errorf("submit forensic report failed after %d retries: %w", c.maxRetries, lastErr)
}

// postForensicReport performs a single POST attempt for a forensic report.
func (c *DynatraceClient) postForensicReport(ctx context.Context, report ForensicReport) error {
	reqCtx, cancel := context.WithTimeout(ctx, c.discoveryTimeout)
	defer cancel()

	body, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("marshaling report: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/reports", c.endpointURL)
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("Dynatrace MCP Server returned status %d", resp.StatusCode)
	}
	return nil
}
