package ollinai

import "encoding/json"

// MaxPayloadBytes is the maximum allowed size for a serialized JSON payload (64 KB).
const MaxPayloadBytes = 65536

// DeploymentRiskPayload is the JSON payload for deployment_risk events.
type DeploymentRiskPayload struct {
	Service     string   `json:"service"`
	CommitSHA   string   `json:"commit_sha"`
	Deployer    string   `json:"deployer"`
	RiskScore   int      `json:"risk_score"`
	RiskFactors []string `json:"risk_factors"`
	PipelineID  string   `json:"pipeline_id"`
	Environment string   `json:"environment"`
}

// DORAMetricsPayload is the JSON payload for dora_metrics events.
type DORAMetricsPayload struct {
	DeploymentFrequency  float64 `json:"deployment_frequency"`
	LeadTimeForChanges   float64 `json:"lead_time_for_changes"`
	ChangeFailureRate    float64 `json:"change_failure_rate"`
	TimeToRestoreService float64 `json:"time_to_restore_service"`
}

// IncidentCorrelationPayload is the JSON payload for incident_correlation events.
type IncidentCorrelationPayload struct {
	DeploymentID string `json:"deployment_id"`
	IncidentID   string `json:"incident_id"`
	Service      string `json:"service"`
	Confidence   int    `json:"confidence"`
	Summary      string `json:"summary"`
}

// SupplyChainPayload is the JSON payload for supply chain eBPF events.
type SupplyChainPayload struct {
	PipelineID  string `json:"pipeline_id"`
	StepName    string `json:"step_name"`
	Repository  string `json:"repository"`
	Description string `json:"description"`
	Evidence    string `json:"evidence,omitempty"`
}

// SerializePayload serializes a payload struct to JSON. If the serialized result
// exceeds MaxPayloadBytes, it attempts truncation by removing optional detail
// fields while preserving required summary fields.
//
// Returns the serialized bytes, a boolean indicating whether truncation occurred,
// and any error encountered during serialization.
func SerializePayload(payload any) ([]byte, bool, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, false, err
	}

	if len(data) <= MaxPayloadBytes {
		return data, false, nil
	}

	// Truncation: attempt to reduce payload size by removing optional fields.
	truncated := truncatePayload(payload)
	data, err = json.Marshal(truncated)
	if err != nil {
		return nil, false, err
	}

	// If still too large after field removal, hard-truncate the JSON bytes.
	if len(data) > MaxPayloadBytes {
		data = data[:MaxPayloadBytes]
	}

	return data, true, nil
}

// truncatePayload returns a reduced version of the payload with optional detail
// fields removed while preserving required summary fields.
func truncatePayload(payload any) any {
	switch p := payload.(type) {
	case DeploymentRiskPayload:
		// RiskFactors is the optional detail field; preserve summary fields.
		return DeploymentRiskPayload{
			Service:     p.Service,
			CommitSHA:   p.CommitSHA,
			Deployer:    p.Deployer,
			RiskScore:   p.RiskScore,
			RiskFactors: nil,
			PipelineID:  p.PipelineID,
			Environment: p.Environment,
		}
	case *DeploymentRiskPayload:
		return &DeploymentRiskPayload{
			Service:     p.Service,
			CommitSHA:   p.CommitSHA,
			Deployer:    p.Deployer,
			RiskScore:   p.RiskScore,
			RiskFactors: nil,
			PipelineID:  p.PipelineID,
			Environment: p.Environment,
		}
	case IncidentCorrelationPayload:
		// Summary is the optional detail field; preserve IDs and confidence.
		return IncidentCorrelationPayload{
			DeploymentID: p.DeploymentID,
			IncidentID:   p.IncidentID,
			Service:      p.Service,
			Confidence:   p.Confidence,
			Summary:      truncateString(p.Summary, 256),
		}
	case *IncidentCorrelationPayload:
		return &IncidentCorrelationPayload{
			DeploymentID: p.DeploymentID,
			IncidentID:   p.IncidentID,
			Service:      p.Service,
			Confidence:   p.Confidence,
			Summary:      truncateString(p.Summary, 256),
		}
	case SupplyChainPayload:
		// Evidence is the optional detail field.
		return SupplyChainPayload{
			PipelineID:  p.PipelineID,
			StepName:    p.StepName,
			Repository:  p.Repository,
			Description: truncateString(p.Description, 512),
			Evidence:    "",
		}
	case *SupplyChainPayload:
		return &SupplyChainPayload{
			PipelineID:  p.PipelineID,
			StepName:    p.StepName,
			Repository:  p.Repository,
			Description: truncateString(p.Description, 512),
			Evidence:    "",
		}
	default:
		// For unknown types (including DORAMetricsPayload which is always small),
		// return as-is; the caller will hard-truncate if needed.
		return payload
	}
}

// truncateString truncates s to maxLen bytes if it exceeds that length.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
