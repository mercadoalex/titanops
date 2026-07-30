package ebeecontrol

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"
)

// TrainerConfig configures the AI trainer behavior.
type TrainerConfig struct {
	// RetrainingInterval is the minimum time between retraining cycles. Range: 1h-168h.
	RetrainingInterval time.Duration
	// MinimumOutcomeRecords is the minimum dataset size before retraining is allowed.
	MinimumOutcomeRecords int
}

// ModelVersion represents a trained model version with its metadata.
type ModelVersion struct {
	VersionID           string    `json:"version_id"`
	TrainingDatasetSize int       `json:"training_dataset_size"`
	ValidationAccuracy  float64   `json:"validation_accuracy"` // percentage 0-100
	PublishedTimestamp  time.Time `json:"published_timestamp"`
}

// TrainingStatus reports the current state of the training pipeline.
type TrainingStatus struct {
	LastRetrainingTimestamp      time.Time     `json:"last_retraining_timestamp"`
	NextScheduledRetraining      time.Time     `json:"next_scheduled_retraining"`
	DatasetSizeSinceLastTraining int           `json:"dataset_size_since_last_training"`
	MinimumRecordsRequired       int           `json:"minimum_records_required"`
	RetrainingInterval           time.Duration `json:"retraining_interval"`
}

// IngestionConfirmation is returned after successful outcome data ingestion.
type IngestionConfirmation struct {
	DatasetEntryCount  int       `json:"dataset_entry_count"`
	IngestionTimestamp time.Time `json:"ingestion_timestamp"`
}

// RetrainingResult contains the outcome of a retraining attempt.
type RetrainingResult struct {
	Success  bool          `json:"success"`
	NewModel *ModelVersion `json:"new_model,omitempty"`
	Reason   string        `json:"reason"`
}

// TrainingLogEvent represents the type of training log entry.
type TrainingLogEvent string

const (
	LogIngestion         TrainingLogEvent = "ingestion"
	LogRetrainingStarted TrainingLogEvent = "retraining_started"
	LogRetrainingSuccess TrainingLogEvent = "retraining_success"
	LogRetrainingFailed  TrainingLogEvent = "retraining_failed"
	LogModelPublished    TrainingLogEvent = "model_published"
	LogModelRejected     TrainingLogEvent = "model_rejected"
)

// TrainingLogEntry records a training event.
type TrainingLogEntry struct {
	Timestamp time.Time        `json:"timestamp"`
	Event     TrainingLogEvent `json:"event"`
	Details   string           `json:"details"`
}

// Trainer manages outcome data ingestion, model retraining scheduling,
// and model publishing with the publish guard (new model only if accuracy >= current).
type Trainer struct {
	mu sync.RWMutex

	config                   TrainerConfig
	currentModel             ModelVersion
	outcomeDataset           []OutcomeData
	datasetSinceLastTraining int
	lastRetrainingTimestamp  time.Time
	logs                     []TrainingLogEntry
}

// NewTrainer creates a Trainer with the given configuration.
// If initialModel is nil, a baseline model (v1.0.0, 75% accuracy) is used.
func NewTrainer(cfg TrainerConfig, initialModel *ModelVersion) (*Trainer, error) {
	if cfg.RetrainingInterval < 1*time.Hour || cfg.RetrainingInterval > 168*time.Hour {
		return nil, fmt.Errorf("retraining interval must be between 1h and 168h, got %s", cfg.RetrainingInterval)
	}
	if cfg.MinimumOutcomeRecords < 1 {
		cfg.MinimumOutcomeRecords = 50
	}

	model := ModelVersion{
		VersionID:           "v1.0.0",
		TrainingDatasetSize: 0,
		ValidationAccuracy:  75,
		PublishedTimestamp:  time.Now().UTC(),
	}
	if initialModel != nil {
		model = *initialModel
	}

	return &Trainer{
		config:                  cfg,
		currentModel:            model,
		lastRetrainingTimestamp: model.PublishedTimestamp,
	}, nil
}

// IngestOutcomeData validates and appends outcome data to the training dataset.
// Returns an IngestionConfirmation on success or an error if validation fails.
func (t *Trainer) IngestOutcomeData(data OutcomeData) (IngestionConfirmation, error) {
	if err := t.validateOutcomeData(data); err != nil {
		return IngestionConfirmation{}, err
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.outcomeDataset = append(t.outcomeDataset, data)
	t.datasetSinceLastTraining++

	confirmation := IngestionConfirmation{
		DatasetEntryCount:  len(t.outcomeDataset),
		IngestionTimestamp: time.Now().UTC(),
	}

	t.addLog(LogIngestion, fmt.Sprintf(
		"Ingested outcome data for incident %s. Dataset size: %d",
		data.IncidentID, len(t.outcomeDataset),
	))

	return confirmation, nil
}

// GetCurrentModel returns the currently deployed model version.
func (t *Trainer) GetCurrentModel() ModelVersion {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.currentModel
}

// GetTrainingStatus returns the current training pipeline status.
func (t *Trainer) GetTrainingStatus() TrainingStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()

	nextRetraining := t.lastRetrainingTimestamp.Add(t.config.RetrainingInterval)

	return TrainingStatus{
		LastRetrainingTimestamp:      t.lastRetrainingTimestamp,
		NextScheduledRetraining:      nextRetraining,
		DatasetSizeSinceLastTraining: t.datasetSinceLastTraining,
		MinimumRecordsRequired:       t.config.MinimumOutcomeRecords,
		RetrainingInterval:           t.config.RetrainingInterval,
	}
}

// ShouldRetrain checks if retraining conditions are met:
//  1. Retraining interval has elapsed since last training
//  2. At least MinimumOutcomeRecords exist since last training
func (t *Trainer) ShouldRetrain() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	intervalElapsed := time.Since(t.lastRetrainingTimestamp) >= t.config.RetrainingInterval
	hasMinRecords := t.datasetSinceLastTraining >= t.config.MinimumOutcomeRecords

	return intervalElapsed && hasMinRecords
}

// TriggerRetraining manually triggers retraining if conditions are met.
//
// The retraining logic:
//  1. Checks minimum outcome records exist since last training
//  2. Simulates model training (generates accuracy between 70-99%)
//  3. Compares new accuracy with current model accuracy
//  4. Publishes only if new >= current (Model Publish Guard)
//  5. On lower accuracy: retains existing model, logs rejection
func (t *Trainer) TriggerRetraining() RetrainingResult {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Check minimum records requirement.
	if t.datasetSinceLastTraining < t.config.MinimumOutcomeRecords {
		reason := fmt.Sprintf(
			"Insufficient outcome records: %d < %d required",
			t.datasetSinceLastTraining, t.config.MinimumOutcomeRecords,
		)
		t.addLog(LogRetrainingFailed, reason)
		return RetrainingResult{Success: false, Reason: reason}
	}

	t.addLog(LogRetrainingStarted, fmt.Sprintf(
		"Starting retraining with %d new records", t.datasetSinceLastTraining,
	))

	// Simulate model training — generate accuracy between 70-99%.
	newAccuracy := float64(rand.Intn(30) + 70)
	currentAccuracy := t.currentModel.ValidationAccuracy

	// Model Publish Guard: publish only if new accuracy >= current.
	if newAccuracy >= currentAccuracy {
		newModel := ModelVersion{
			VersionID:           t.generateVersionID(),
			TrainingDatasetSize: len(t.outcomeDataset),
			ValidationAccuracy:  newAccuracy,
			PublishedTimestamp:  time.Now().UTC(),
		}

		t.currentModel = newModel
		t.lastRetrainingTimestamp = newModel.PublishedTimestamp
		t.datasetSinceLastTraining = 0

		t.addLog(LogModelPublished, fmt.Sprintf(
			"New model %s published. Accuracy: %.0f%% (was %.0f%%)",
			newModel.VersionID, newAccuracy, currentAccuracy,
		))

		return RetrainingResult{
			Success:  true,
			NewModel: &newModel,
			Reason:   fmt.Sprintf("New model accuracy %.0f%% >= current %.0f%%", newAccuracy, currentAccuracy),
		}
	}

	// New model is worse — retain existing.
	reason := fmt.Sprintf(
		"New model accuracy %.0f%% < current %.0f%%. Retaining existing model.",
		newAccuracy, currentAccuracy,
	)
	t.addLog(LogModelRejected, reason)
	return RetrainingResult{Success: false, Reason: reason}
}

// GetLogs returns all training log entries.
func (t *Trainer) GetLogs() []TrainingLogEntry {
	t.mu.RLock()
	defer t.mu.RUnlock()
	logs := make([]TrainingLogEntry, len(t.logs))
	copy(logs, t.logs)
	return logs
}

// GetDatasetSize returns the total number of outcome records ingested.
func (t *Trainer) GetDatasetSize() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.outcomeDataset)
}

// validateOutcomeData checks outcome data for completeness.
func (t *Trainer) validateOutcomeData(data OutcomeData) error {
	if strings.TrimSpace(data.IncidentID) == "" {
		return fmt.Errorf("outcome data missing required field: incidentId")
	}
	if strings.TrimSpace(data.AccessEvent.EventID) == "" {
		return fmt.Errorf("outcome data missing required field: accessEvent.eventId")
	}
	if data.AccessEvent.ProcessID < 0 {
		return fmt.Errorf("outcome data missing required field: accessEvent.processId")
	}
	if strings.TrimSpace(data.AccessEvent.ProcessBinaryPath) == "" {
		return fmt.Errorf("outcome data missing required field: accessEvent.processBinaryPath")
	}
	if data.AccessEvent.UserID < 0 {
		return fmt.Errorf("outcome data missing required field: accessEvent.userId")
	}
	if strings.TrimSpace(data.AccessEvent.PodID) == "" {
		return fmt.Errorf("outcome data missing required field: accessEvent.podId")
	}
	if strings.TrimSpace(data.AccessEvent.Namespace) == "" {
		return fmt.Errorf("outcome data missing required field: accessEvent.namespace")
	}
	if strings.TrimSpace(data.AccessEvent.HoneytokenPath) == "" {
		return fmt.Errorf("outcome data missing required field: accessEvent.honeytokenPath")
	}

	validTypes := map[AccessType]bool{
		AccessOpen: true, AccessRead: true, AccessWrite: true, AccessStat: true,
	}
	if !validTypes[data.AccessEvent.AccessType] {
		return fmt.Errorf("outcome data missing required field: accessEvent.accessType")
	}
	if data.AccessEvent.Timestamp.IsZero() {
		return fmt.Errorf("outcome data missing required field: accessEvent.timestamp")
	}

	validHoneytokenTypes := map[HoneytokenType]bool{
		HoneytokenDecoySecret: true, HoneytokenDecoyFile: true, HoneytokenDecoyCredential: true,
	}
	if !validHoneytokenTypes[data.HoneytokenType] {
		return fmt.Errorf("outcome data missing required field: honeytokenType")
	}
	if strings.TrimSpace(data.PlacementLocation) == "" {
		return fmt.Errorf("outcome data missing required field: placementLocation")
	}
	if len(data.ActionsTaken) == 0 {
		return fmt.Errorf("outcome data missing required field: actionsTaken (must have at least one)")
	}
	if data.Effectiveness.DetectionToResponseLatency < 0 {
		return fmt.Errorf("outcome data missing required field: effectiveness.detectionToResponseLatency")
	}
	if data.Timestamp.IsZero() {
		return fmt.Errorf("outcome data missing required field: timestamp")
	}

	return nil
}

// generateVersionID creates a version string based on dataset size and timestamp.
func (t *Trainer) generateVersionID() string {
	major := len(t.outcomeDataset)/100 + 1
	minor := len(t.outcomeDataset) % 100
	return fmt.Sprintf("v%d.%d.%d", major, minor, time.Now().UnixMilli()%10000)
}

// addLog appends a training log entry (must be called with lock held).
func (t *Trainer) addLog(event TrainingLogEvent, details string) {
	t.logs = append(t.logs, TrainingLogEntry{
		Timestamp: time.Now().UTC(),
		Event:     event,
		Details:   details,
	})
}
