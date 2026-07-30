// Package ebeecontrol implements an autonomous deception engine for Kubernetes
// that deploys honeytokens, monitors access via eBPF (Tetragon), classifies threats,
// and responds autonomously with pod isolation, IP blocking, and adaptive learning.
package ebeecontrol

import "time"

// AccessType represents a kernel-level file access operation detected by Tetragon.
type AccessType string

const (
	AccessOpen  AccessType = "open"
	AccessRead  AccessType = "read"
	AccessWrite AccessType = "write"
	AccessStat  AccessType = "stat"
)

// ThreatClassification represents the severity level assigned to a honeytoken access event.
type ThreatClassification string

const (
	ThreatLow      ThreatClassification = "low"
	ThreatMedium   ThreatClassification = "medium"
	ThreatHigh     ThreatClassification = "high"
	ThreatCritical ThreatClassification = "critical"
)

// HoneytokenType represents the category of a deployed honeytoken.
type HoneytokenType string

const (
	HoneytokenDecoySecret     HoneytokenType = "decoy_secret"
	HoneytokenDecoyFile       HoneytokenType = "decoy_file"
	HoneytokenDecoyCredential HoneytokenType = "decoy_credential"
)

// HoneytokenStatus represents the lifecycle state of a deployed honeytoken.
type HoneytokenStatus string

const (
	HoneytokenActive         HoneytokenStatus = "active"
	HoneytokenTriggered      HoneytokenStatus = "triggered"
	HoneytokenDecommissioned HoneytokenStatus = "decommissioned"
)

// ActionType represents an autonomous response action.
type ActionType string

const (
	ActionPodIsolation          ActionType = "pod_isolation"
	ActionIPBlock               ActionType = "ip_block"
	ActionAdditionalHoneytokens ActionType = "additional_honeytokens"
	ActionAlert                 ActionType = "alert"
)

// ActionResult represents the outcome of a response action.
type ActionResult string

const (
	ActionSuccess ActionResult = "success"
	ActionFailure ActionResult = "failure"
)

// NamespaceClassification indicates whether a namespace is production or not.
type NamespaceClassification string

const (
	NamespaceProduction    NamespaceClassification = "production"
	NamespaceNonProduction NamespaceClassification = "non-production"
)

// DecisionType represents the category of an autonomous decision.
type DecisionType string

const (
	DecisionDiscovery   DecisionType = "discovery"
	DecisionDeployment  DecisionType = "deployment"
	DecisionAssessment  DecisionType = "assessment"
	DecisionResponse    DecisionType = "response"
	DecisionLearning    DecisionType = "learning"
	DecisionModelUpdate DecisionType = "model_update"
)

// AccessEvent represents a kernel-level file access event detected by Tetragon.
// Generated when a process performs an operation on a deployed honeytoken path.
type AccessEvent struct {
	EventID           string     `json:"event_id"`
	ProcessID         int        `json:"process_id"`
	ProcessBinaryPath string     `json:"process_binary_path"`
	UserID            int        `json:"user_id"`
	PodID             string     `json:"pod_id"`
	Namespace         string     `json:"namespace"`
	HoneytokenPath    string     `json:"honeytoken_path"`
	AccessType        AccessType `json:"access_type"`
	Timestamp         time.Time  `json:"timestamp"`
}

// HighRiskService is a Kubernetes service identified by Dynatrace as having
// elevated vulnerability or exposure. Used during discovery to select targets.
type HighRiskService struct {
	ServiceID      string   `json:"service_id"`
	ServiceName    string   `json:"service_name"`
	Namespace      string   `json:"namespace"`
	PodIdentifiers []string `json:"pod_identifiers"`
	RiskScore      float64  `json:"risk_score"` // 0-100
}

// PodContext contains contextual information about a pod from Dynatrace.
// Used for threat classification during the assessment phase.
type PodContext struct {
	Namespace               string                  `json:"namespace"`
	NamespaceClassification NamespaceClassification `json:"namespace_classification"`
	ServiceCriticality      int                     `json:"service_criticality"`    // 1-5
	DavisAnomalyScore       float64                 `json:"davis_anomaly_score"`    // 0.0-1.0
	AnomalyWindowMinutes    int                     `json:"anomaly_window_minutes"` // default 10
}

// ThreatAssessment is the result of contextual threat classification for an access event.
type ThreatAssessment struct {
	AssessmentID      string               `json:"assessment_id"`
	AccessEventID     string               `json:"access_event_id"`
	Classification    ThreatClassification `json:"classification"`
	Inputs            ThreatInputs         `json:"inputs"`
	AssessmentTime    time.Time            `json:"assessment_timestamp"`
	AssessmentLatency time.Duration        `json:"assessment_latency_ms"`
}

// ThreatInputs contains the raw inputs used for threat classification.
type ThreatInputs struct {
	NamespaceClassification NamespaceClassification `json:"namespace_classification"`
	ServiceCriticality      int                     `json:"service_criticality"`
	DavisAnomalyScore       float64                 `json:"davis_anomaly_score"`
}

// HoneytokenRegistryEntry tracks a deployed honeytoken through its lifecycle.
type HoneytokenRegistryEntry struct {
	HoneytokenID        string           `json:"honeytoken_id"`
	PodID               string           `json:"pod_id"`
	Namespace           string           `json:"namespace"`
	Type                HoneytokenType   `json:"type"`
	FilePath            string           `json:"file_path"`
	DeploymentTimestamp time.Time        `json:"deployment_timestamp"`
	Status              HoneytokenStatus `json:"status"`
	LastAccessTimestamp *time.Time       `json:"last_access_timestamp,omitempty"`
	AccessCount         int              `json:"access_count"`
}

// ResponseAction tracks a threat response action executed by the agent.
type ResponseAction struct {
	ActionID             string               `json:"action_id"`
	ActionType           ActionType           `json:"action_type"`
	Target               string               `json:"target"`
	Timestamp            time.Time            `json:"timestamp"`
	ThreatClassification ThreatClassification `json:"threat_classification"`
	Result               ActionResult         `json:"result"`
	RetryCount           int                  `json:"retry_count"`
}

// ForensicReport is a structured forensic report generated after a threat response.
type ForensicReport struct {
	ReportID             string              `json:"report_id"`
	GenerationTimestamp  time.Time           `json:"generation_timestamp"`
	TriggeringEventID    string              `json:"triggering_access_event_id"`
	RetentionDays        int                 `json:"retention_days"`
	AccessEventDetails   ForensicAccessEvent `json:"access_event_details"`
	ContextualAssessment ForensicAssessment  `json:"contextual_assessment"`
	ResponseActions      []ForensicAction    `json:"response_actions"`
	Timeline             []TimelineEntry     `json:"timeline"`
	RecommendedActions   []string            `json:"recommended_follow_up_actions"`
}

// ForensicAccessEvent contains access event details within a forensic report.
type ForensicAccessEvent struct {
	ProcessID      int        `json:"process_id"`
	UserID         int        `json:"user_id"`
	PodID          string     `json:"pod_id"`
	Namespace      string     `json:"namespace"`
	HoneytokenPath string     `json:"honeytoken_path"`
	AccessType     AccessType `json:"access_type"`
	Timestamp      time.Time  `json:"timestamp"`
}

// ForensicAssessment contains the threat assessment within a forensic report.
type ForensicAssessment struct {
	ThreatClassification ThreatClassification `json:"threat_classification"`
	PodCriticality       int                  `json:"pod_criticality"`
	AnomalyScore         float64              `json:"anomaly_score"`
}

// ForensicAction records a response action within a forensic report.
type ForensicAction struct {
	ActionType string       `json:"action_type"`
	Target     string       `json:"target"`
	Timestamp  time.Time    `json:"timestamp"`
	Result     ActionResult `json:"result"`
}

// TimelineEntry is a single event in a forensic report's timeline.
type TimelineEntry struct {
	EventDescription string    `json:"event_description"`
	Timestamp        time.Time `json:"timestamp"`
}

// AuditLogEntry records an autonomous decision for accountability.
type AuditLogEntry struct {
	EntryID           string       `json:"entry_id"`
	Timestamp         time.Time    `json:"timestamp"`
	DecisionType      DecisionType `json:"decision_type"`
	DecisionRationale string       `json:"decision_rationale"`
	InputDataSummary  string       `json:"input_data_summary"`
	Outcome           string       `json:"outcome"`
	RetentionDays     int          `json:"retention_days"`
}

// PlacementModel represents a trained placement optimization model.
type PlacementModel struct {
	VersionID           string    `json:"version_id"`
	TrainingDatasetSize int       `json:"training_dataset_size"`
	ValidationAccuracy  float64   `json:"validation_accuracy"` // percentage 0-100
	PublishedTimestamp  time.Time `json:"published_timestamp"`
	ModelArtifactURI    string    `json:"model_artifact_uri"`
}

// OutcomeData is submitted to the AI trainer after a threat response sequence.
type OutcomeData struct {
	IncidentID        string           `json:"incident_id"`
	AccessEvent       AccessEvent      `json:"access_event"`
	HoneytokenType    HoneytokenType   `json:"honeytoken_type"`
	PlacementLocation string           `json:"placement_location"`
	ActionsTaken      []ResponseAction `json:"actions_taken"`
	Effectiveness     Effectiveness    `json:"effectiveness"`
	Timestamp         time.Time        `json:"timestamp"`
}

// Effectiveness captures the outcome metrics of a response.
type Effectiveness struct {
	DetectionToResponseLatency time.Duration `json:"detection_to_response_latency_seconds"`
	ThreatContained            bool          `json:"threat_contained"`
	FalsePositive              bool          `json:"false_positive"`
}

// DeploymentRequest specifies a honeytoken deployment to a pod.
type DeploymentRequest struct {
	PodID       string           `json:"pod_id"`
	Namespace   string           `json:"namespace"`
	Honeytokens []HoneytokenSpec `json:"honeytokens"`
}

// HoneytokenSpec defines a honeytoken to deploy.
type HoneytokenSpec struct {
	Type      HoneytokenType `json:"type"`
	Name      string         `json:"name"`
	Placement string         `json:"placement"` // file path or secret name
	Content   string         `json:"content,omitempty"`
}

// DeploymentResponse contains the results of a deployment request.
type DeploymentResponse struct {
	Success             bool                 `json:"success"`
	DeployedHoneytokens []DeployedHoneytoken `json:"deployed_honeytokens"`
	Errors              []DeploymentError    `json:"errors"`
}

// DeployedHoneytoken records a successfully deployed honeytoken.
type DeployedHoneytoken struct {
	HoneytokenID        string         `json:"honeytoken_id"`
	PodID               string         `json:"pod_id"`
	Namespace           string         `json:"namespace"`
	Type                HoneytokenType `json:"type"`
	FilePath            string         `json:"file_path"`
	DeploymentTimestamp time.Time      `json:"deployment_timestamp"`
}

// DeploymentError contains details about a failed deployment.
type DeploymentError struct {
	PodID              string   `json:"pod_id"`
	FailureReason      string   `json:"failure_reason"`
	RemediationActions []string `json:"remediation_actions"`
}

// DeploymentStatus indicates the current state of a deployed honeytoken.
type DeploymentStatus string

const (
	DeploymentActive         DeploymentStatus = "active"
	DeploymentTriggered      DeploymentStatus = "triggered"
	DeploymentDecommissioned DeploymentStatus = "decommissioned"
	DeploymentNotFound       DeploymentStatus = "not_found"
)
