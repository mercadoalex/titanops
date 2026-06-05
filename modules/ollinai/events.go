package ollinai

// ModuleName is the constant module identifier for OllinAI events.
const ModuleName = "ollinai"

// Event type constants for OllinAI events published to the NATS event bus.
const (
	// EventTypeDeploymentRisk is emitted when OllinAI computes a deployment risk score.
	EventTypeDeploymentRisk = "deployment_risk"

	// EventTypeDORAMetrics is emitted when OllinAI computes updated DORA metrics.
	EventTypeDORAMetrics = "dora_metrics"

	// EventTypeIncidentCorrelation is emitted when OllinAI detects a deployment-incident correlation.
	EventTypeIncidentCorrelation = "incident_correlation"

	// EventTypeSupplyChainCredentialExfil is emitted when the eBPF agent detects credential exfiltration.
	EventTypeSupplyChainCredentialExfil = "supply_chain_credential_exfil"

	// EventTypeSupplyChainProcessAnomaly is emitted when the eBPF agent detects unauthorized process ancestry.
	EventTypeSupplyChainProcessAnomaly = "supply_chain_process_anomaly"

	// EventTypeSupplyChainAttestationFailure is emitted when the eBPF agent detects a build attestation failure.
	EventTypeSupplyChainAttestationFailure = "supply_chain_attestation_failure"
)

// Label key constants used in event Labels.
const (
	// LabelService is the target service name.
	LabelService = "service"
	// LabelCommitSHA is the git commit hash.
	LabelCommitSHA = "commit_sha"
	// LabelDeployer is the deploying user or system.
	LabelDeployer = "deployer"
	// LabelPipelineID is the CI/CD pipeline identifier.
	LabelPipelineID = "pipeline_id"
	// LabelStepName is the pipeline step name.
	LabelStepName = "step_name"
	// LabelRepository is the source repository.
	LabelRepository = "repository"
	// LabelMetadataIncomplete is set to "true" when Node, Pod, or Namespace is missing.
	LabelMetadataIncomplete = "metadata_incomplete"
	// LabelPayloadTruncated is set to "true" when the payload was truncated to fit 64KB.
	LabelPayloadTruncated = "payload_truncated"
)
