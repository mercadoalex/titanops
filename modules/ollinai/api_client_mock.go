package ollinai

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
)

// MockAPIClient is a deterministic in-memory implementation of OllinAPIClient.
// Used in mock mode (TITANOPS_MODE=mock) and in tests. Given the same seed,
// it produces identical outputs across runs.
type MockAPIClient struct {
	mu   sync.Mutex
	rng  *rand.Rand
	seed int64

	// CallCount tracks how many times each method was called.
	CallCount MockAPICallCount

	// FailNext controls error injection for testing. When set to a non-nil
	// APIError, the next call to the specified method returns that error
	// and resets FailNext to nil.
	FailNextRisk *APIError
	FailNextDORA *APIError
}

// MockAPICallCount tracks invocation counts for observability in tests.
type MockAPICallCount struct {
	FetchDeploymentRisks int
	FetchDORAMetrics     int
}

// MockAPIClientConfig configures the mock client.
type MockAPIClientConfig struct {
	// Seed controls deterministic random generation. Default: 42.
	Seed int64
}

// NewMockAPIClient creates a deterministic mock OllinAI API client.
func NewMockAPIClient(cfg MockAPIClientConfig) *MockAPIClient {
	seed := cfg.Seed
	if seed == 0 {
		seed = 42
	}
	return &MockAPIClient{
		rng:  rand.New(rand.NewSource(seed)),
		seed: seed,
	}
}

// FetchDeploymentRisks returns deterministic synthetic deployment risk entries.
func (m *MockAPIClient) FetchDeploymentRisks(ctx context.Context) ([]DeploymentRiskEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CallCount.FetchDeploymentRisks++

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	if m.FailNextRisk != nil {
		err := m.FailNextRisk
		m.FailNextRisk = nil
		return nil, err
	}

	// Generate 1-5 synthetic entries deterministically
	count := m.rng.Intn(5) + 1
	entries := make([]DeploymentRiskEntry, count)

	services := []string{"api-gateway", "payment-service", "user-service", "inventory-service", "notification-service"}
	environments := []string{"production", "staging", "canary"}
	factors := []string{"large_diff", "no_tests", "new_contributor", "hotfix", "schema_change", "dependency_update"}

	for i := 0; i < count; i++ {
		numFactors := m.rng.Intn(4) + 1
		riskFactors := make([]string, numFactors)
		for j := range riskFactors {
			riskFactors[j] = factors[m.rng.Intn(len(factors))]
		}

		entries[i] = DeploymentRiskEntry{
			Service:     services[m.rng.Intn(len(services))],
			CommitSHA:   fmt.Sprintf("%040x", m.rng.Int63()),
			Deployer:    fmt.Sprintf("user-%d", m.rng.Intn(20)),
			RiskScore:   m.rng.Intn(101),
			RiskFactors: riskFactors,
			PipelineID:  fmt.Sprintf("pipe-%06d", m.rng.Intn(100000)),
			Environment: environments[m.rng.Intn(len(environments))],
			Node:        fmt.Sprintf("node-%02d", m.rng.Intn(10)),
			Pod:         fmt.Sprintf("pod-%s-%05d", services[i%len(services)], m.rng.Intn(99999)),
			Namespace:   "titanops",
		}
	}

	return entries, nil
}

// FetchDORAMetrics returns deterministic synthetic DORA metrics.
func (m *MockAPIClient) FetchDORAMetrics(ctx context.Context) (*DORAMetrics, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CallCount.FetchDORAMetrics++

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	if m.FailNextDORA != nil {
		err := m.FailNextDORA
		m.FailNextDORA = nil
		return nil, err
	}

	return &DORAMetrics{
		DeploymentFrequency:  m.rng.Float64() * 10,    // 0-10 deploys/day
		LeadTimeForChanges:   m.rng.Float64() * 72,    // 0-72 hours
		ChangeFailureRate:    m.rng.Float64() * 0.5,   // 0-50%
		TimeToRestoreService: m.rng.Float64()*4 + 0.1, // 0.1-4.1 hours
	}, nil
}

// Reset re-initializes the mock with the original seed, clearing call counts
// and error injection. Useful for test isolation.
func (m *MockAPIClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.rng = rand.New(rand.NewSource(m.seed))
	m.CallCount = MockAPICallCount{}
	m.FailNextRisk = nil
	m.FailNextDORA = nil
}
