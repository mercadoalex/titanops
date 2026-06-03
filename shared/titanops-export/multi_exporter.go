package export

import (
	"context"
	"fmt"
	"sync"
)

// Backend represents a single export destination.
type Backend interface {
	// Name returns the identifier for this backend (e.g., "prometheus", "splunk").
	Name() string
	// Send exports an event to this backend.
	Send(ctx context.Context, event Event) error
	// IsEnabled returns whether this backend is currently active.
	IsEnabled() bool
}

// MultiExporter dispatches events to all enabled backends concurrently.
// Failure in one backend does not block or delay delivery to other backends.
type MultiExporter struct {
	backends []Backend
	buffers  map[string]*bufferState // per-backend buffer state
}

// bufferState tracks buffer utilization for a single backend.
// Full ring buffer integration comes in task 5.3; this provides the reporting interface.
type bufferState struct {
	capacity int
	used     int
	dropped  int
}

// NewMultiExporter creates an exporter that dispatches to the given backends.
func NewMultiExporter(backends ...Backend) *MultiExporter {
	buffers := make(map[string]*bufferState, len(backends))
	for _, b := range backends {
		buffers[b.Name()] = &bufferState{
			capacity: 1000,
			used:     0,
			dropped:  0,
		}
	}
	return &MultiExporter{
		backends: backends,
		buffers:  buffers,
	}
}

// Export sends an event to all enabled backends concurrently.
// Each backend runs in its own goroutine. Failure in one does not affect others.
// Returns a result for each enabled backend.
func (m *MultiExporter) Export(ctx context.Context, event Event) []ExportResult {
	enabledBackends := m.enabledBackends()
	if len(enabledBackends) == 0 {
		return nil
	}

	results := make([]ExportResult, len(enabledBackends))
	var wg sync.WaitGroup
	wg.Add(len(enabledBackends))

	for i, b := range enabledBackends {
		go func(idx int, backend Backend) {
			defer wg.Done()
			results[idx] = m.sendToBackend(ctx, backend, event)
		}(i, b)
	}

	wg.Wait()
	return results
}

// sendToBackend sends an event to a single backend with panic recovery.
// It never panics — any panic from the backend is recovered and returned as an error.
func (m *MultiExporter) sendToBackend(ctx context.Context, backend Backend, event Event) (result ExportResult) {
	result.Backend = backend.Name()

	// Recover from panics in backend.Send
	defer func() {
		if r := recover(); r != nil {
			result.Success = false
			result.Error = &BackendPanicError{
				BackendName: backend.Name(),
				Recovered:   r,
			}
		}
	}()

	err := backend.Send(ctx, event)
	if err != nil {
		result.Success = false
		result.Error = &BackendSendError{
			BackendName: backend.Name(),
			Cause:       err,
		}
	} else {
		result.Success = true
	}

	return result
}

// BufferStatus returns current buffer utilization per backend.
// The map key is the backend identifier (e.g., "prometheus", "splunk").
func (m *MultiExporter) BufferStatus() map[string]BufferInfo {
	status := make(map[string]BufferInfo, len(m.buffers))
	for name, buf := range m.buffers {
		status[name] = BufferInfo{
			Capacity: buf.capacity,
			Used:     buf.used,
			Dropped:  buf.dropped,
		}
	}
	return status
}

// enabledBackends returns only backends that report IsEnabled() == true.
func (m *MultiExporter) enabledBackends() []Backend {
	var enabled []Backend
	for _, b := range m.backends {
		if b.IsEnabled() {
			enabled = append(enabled, b)
		}
	}
	return enabled
}

// BackendPanicError represents a panic recovered from a backend Send operation.
type BackendPanicError struct {
	BackendName string
	Recovered   interface{}
}

func (e *BackendPanicError) Error() string {
	return fmt.Sprintf("backend %q panicked: %v", e.BackendName, e.Recovered)
}

// BackendSendError represents a failure returned by a backend Send operation.
type BackendSendError struct {
	BackendName string
	Cause       error
}

func (e *BackendSendError) Error() string {
	return fmt.Sprintf("backend %q send failed: %v", e.BackendName, e.Cause)
}

func (e *BackendSendError) Unwrap() error {
	return e.Cause
}
