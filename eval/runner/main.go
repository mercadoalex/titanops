// Package main implements the TitanOps evaluation runner.
// It loads YAML scenarios from eval/scenarios/<module>/, executes them against
// the module's scoring logic in mock mode, and produces a JSON report.
//
// Usage:
//
//	go run . --scenarios ../../eval/scenarios/correlation
//	go run . --scenarios ../../eval/scenarios/correlation --report ../../eval/reports/correlation/
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mercadoalex/titanops/correlation"
	export "github.com/mercadoalex/titanops/shared/titanops-export"
	"gopkg.in/yaml.v3"
)

func main() {
	scenariosDir := flag.String("scenarios", "", "Path to scenarios directory (e.g., ../scenarios/correlation)")
	reportDir := flag.String("report", "", "Path to output report directory (optional; prints to stdout if omitted)")
	flag.Parse()

	if *scenariosDir == "" {
		log.Fatal("--scenarios flag is required")
	}

	// Detect module from directory name
	module := filepath.Base(*scenariosDir)
	log.Printf("Eval runner: module=%s scenarios=%s", module, *scenariosDir)

	// Load all YAML scenario files
	scenarios, err := loadScenarios(*scenariosDir)
	if err != nil {
		log.Fatalf("Failed to load scenarios: %v", err)
	}
	log.Printf("Loaded %d scenarios", len(scenarios))

	// Run scenarios based on module
	var results []ScenarioResult
	switch module {
	case "correlation":
		results = runCorrelationScenarios(scenarios)
	default:
		log.Fatalf("Unsupported module: %s (supported: correlation)", module)
	}

	// Build report
	report := EvalReport{
		Module:     module,
		Timestamp:  time.Now().UTC(),
		CommitSHA:  os.Getenv("GIT_COMMIT"),
		TotalRun:   len(results),
		Passed:     countPassed(results),
		Failed:     countFailed(results),
		Scenarios:  results,
	}

	reportJSON, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal report: %v", err)
	}

	// Output report
	if *reportDir != "" {
		if err := os.MkdirAll(*reportDir, 0755); err != nil {
			log.Fatalf("Failed to create report dir: %v", err)
		}
		shortCommit := report.CommitSHA
		if len(shortCommit) > 7 {
			shortCommit = shortCommit[:7]
		}
		if shortCommit == "" {
			shortCommit = "local"
		}
		filename := fmt.Sprintf("%s-%s.json", time.Now().Format("2006-01-02"), shortCommit)
		path := filepath.Join(*reportDir, filename)
		if err := os.WriteFile(path, reportJSON, 0644); err != nil {
			log.Fatalf("Failed to write report: %v", err)
		}
		log.Printf("Report written to %s", path)
	} else {
		fmt.Println(string(reportJSON))
	}

	// Exit with non-zero if any scenario failed (for CI gating)
	if report.Failed > 0 {
		os.Exit(1)
	}
}

// --- Report Types ---

// EvalReport is the structured JSON output of an evaluation run.
type EvalReport struct {
	Module    string           `json:"module"`
	Timestamp time.Time        `json:"timestamp"`
	CommitSHA string           `json:"commit_sha,omitempty"`
	TotalRun  int              `json:"total_run"`
	Passed    int              `json:"passed"`
	Failed    int              `json:"failed"`
	Scenarios []ScenarioResult `json:"scenarios"`
}

// ScenarioResult captures the outcome of a single scenario execution.
type ScenarioResult struct {
	Name     string `json:"name"`
	File     string `json:"file"`
	Passed   bool   `json:"passed"`
	Failures []string `json:"failures,omitempty"`
	// Actual values for debugging
	ActualIncidents  int `json:"actual_incidents,omitempty"`
	ActualConfidence int `json:"actual_confidence,omitempty"`
}

// --- Scenario Schema ---

// CorrelationScenario is the YAML schema for correlation eval scenarios.
type CorrelationScenario struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Inputs      struct {
		Events []CorrelationEventInput `yaml:"events"`
		Config struct {
			TimeWindowSeconds   int `yaml:"time_window_seconds"`
			ConfidenceThreshold int `yaml:"confidence_threshold"`
		} `yaml:"config"`
	} `yaml:"inputs"`
	Expected struct {
		IncidentsGenerated int      `yaml:"incidents_generated"`
		MinConfidence      int      `yaml:"min_confidence"`
		MaxConfidence      int      `yaml:"max_confidence"`
		NarrativeContains  []string `yaml:"narrative_contains"`
	} `yaml:"expected"`
}

// CorrelationEventInput represents an event in a YAML scenario.
type CorrelationEventInput struct {
	Module    string            `yaml:"module"`
	EventType string           `yaml:"event_type"`
	Node      string            `yaml:"node"`
	Pod       string            `yaml:"pod"`
	Namespace string            `yaml:"namespace"`
	Severity  string            `yaml:"severity"`
	Timestamp string            `yaml:"timestamp"`
	Payload   string            `yaml:"payload"`
	Labels    map[string]string `yaml:"labels"`
}

// --- Scenario Loading ---

func loadScenarios(dir string) ([]scenarioFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", dir, err)
	}

	var scenarios []scenarioFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		scenarios = append(scenarios, scenarioFile{name: name, path: path, data: data})
	}
	return scenarios, nil
}

type scenarioFile struct {
	name string
	path string
	data []byte
}

// --- Correlation Runner ---

func runCorrelationScenarios(files []scenarioFile) []ScenarioResult {
	var results []ScenarioResult

	for _, f := range files {
		var scenario CorrelationScenario
		if err := yaml.Unmarshal(f.data, &scenario); err != nil {
			results = append(results, ScenarioResult{
				Name:     f.name,
				File:     f.path,
				Passed:   false,
				Failures: []string{fmt.Sprintf("YAML parse error: %v", err)},
			})
			continue
		}

		result := executeCorrelationScenario(scenario, f.path)
		results = append(results, result)
	}

	return results
}

func executeCorrelationScenario(scenario CorrelationScenario, filePath string) ScenarioResult {
	result := ScenarioResult{
		Name: scenario.Name,
		File: filePath,
	}

	// Build correlation engine with scenario config
	cfg := correlation.EngineConfig{
		TimeWindow:          time.Duration(scenario.Inputs.Config.TimeWindowSeconds) * time.Second,
		ConfidenceThreshold: scenario.Inputs.Config.ConfidenceThreshold,
		AutoActions: []correlation.AutoActionConfig{
			{Type: "isolate_pod", Enabled: true},
		},
	}

	engine, err := correlation.NewEngine(cfg, nil, &correlation.NoOpExecutor{})
	if err != nil {
		result.Failures = append(result.Failures, fmt.Sprintf("engine creation failed: %v", err))
		return result
	}

	// Ingest events
	ctx := context.Background()
	for _, ev := range scenario.Inputs.Events {
		ts, _ := time.Parse(time.RFC3339, ev.Timestamp)
		event := export.Event{
			Module:    ev.Module,
			EventType: ev.EventType,
			Node:      ev.Node,
			Pod:       ev.Pod,
			Namespace: ev.Namespace,
			Severity:  ev.Severity,
			Timestamp: ts,
			Payload:   []byte(ev.Payload),
			Labels:    ev.Labels,
		}
		if err := engine.Ingest(ctx, event); err != nil {
			result.Failures = append(result.Failures, fmt.Sprintf("ingest failed: %v", err))
			return result
		}
	}

	// Run correlation
	incidents, err := engine.Correlate(ctx)
	if err != nil {
		result.Failures = append(result.Failures, fmt.Sprintf("correlate failed: %v", err))
		return result
	}

	result.ActualIncidents = len(incidents)

	// Check: incidents_generated
	if len(incidents) != scenario.Expected.IncidentsGenerated {
		result.Failures = append(result.Failures,
			fmt.Sprintf("incidents_generated: expected %d, got %d",
				scenario.Expected.IncidentsGenerated, len(incidents)))
	}

	// Check confidence and narrative (only if incidents were expected)
	if scenario.Expected.IncidentsGenerated > 0 && len(incidents) > 0 {
		inc := incidents[0]
		result.ActualConfidence = inc.ConfidenceScore

		if inc.ConfidenceScore < scenario.Expected.MinConfidence {
			result.Failures = append(result.Failures,
				fmt.Sprintf("min_confidence: expected >= %d, got %d",
					scenario.Expected.MinConfidence, inc.ConfidenceScore))
		}
		if scenario.Expected.MaxConfidence > 0 && inc.ConfidenceScore > scenario.Expected.MaxConfidence {
			result.Failures = append(result.Failures,
				fmt.Sprintf("max_confidence: expected <= %d, got %d",
					scenario.Expected.MaxConfidence, inc.ConfidenceScore))
		}

		// Check narrative contains
		for _, substr := range scenario.Expected.NarrativeContains {
			if !strings.Contains(strings.ToLower(inc.Narrative), strings.ToLower(substr)) {
				result.Failures = append(result.Failures,
					fmt.Sprintf("narrative_contains: expected %q in narrative, got: %s",
						substr, inc.Narrative))
			}
		}
	}

	result.Passed = len(result.Failures) == 0
	return result
}

// --- Helpers ---

func countPassed(results []ScenarioResult) int {
	n := 0
	for _, r := range results {
		if r.Passed {
			n++
		}
	}
	return n
}

func countFailed(results []ScenarioResult) int {
	n := 0
	for _, r := range results {
		if !r.Passed {
			n++
		}
	}
	return n
}
