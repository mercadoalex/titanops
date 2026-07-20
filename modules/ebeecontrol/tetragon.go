package ebeecontrol

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// TetragonKprobeEvent represents a raw kprobe event from the Tetragon gRPC stream.
type TetragonKprobeEvent struct {
	Process      TetragonProcess `json:"process"`
	Args         TetragonArgs    `json:"args"`
	FunctionName string          `json:"function_name"`
	Action       string          `json:"action"`
	Time         time.Time       `json:"time"`
}

// TetragonProcess contains process metadata from a Tetragon event.
type TetragonProcess struct {
	PID    int          `json:"pid"`
	Binary string       `json:"binary"`
	UID    int          `json:"uid"`
	Pod    TetragonPod  `json:"pod"`
}

// TetragonPod contains pod metadata from a Tetragon event.
type TetragonPod struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// TetragonArgs contains kprobe arguments from a Tetragon event.
type TetragonArgs struct {
	FileArg   string `json:"file_arg,omitempty"`
	StringArg string `json:"string_arg,omitempty"`
}

// TetragonStream is an abstraction over the Tetragon gRPC event stream.
// In production, this wraps a grpc.ClientStream from the Tetragon observer API.
type TetragonStream interface {
	// Recv blocks until the next event arrives or the stream ends.
	Recv() (*TetragonKprobeEvent, error)
	// Close terminates the stream.
	Close() error
}

// TetragonConnector creates a TetragonStream connected to the given address.
// Inject this for testing; in production, use a gRPC-based implementation.
type TetragonConnector func(ctx context.Context, address string) (TetragonStream, error)

// TetragonMonitor manages the connection to Tetragon, transforms kprobe events
// into AccessEvents, and maintains a circular event buffer.
type TetragonMonitor struct {
	config    TetragonConfig
	connector TetragonConnector
	logger    *log.Logger

	mu              sync.RWMutex
	registeredPaths map[string]struct{}
	callbacks       []func(AccessEvent)
	buffer          *EventBuffer
	connected       bool
	cancel          context.CancelFunc
	done            chan struct{}
}

// NewTetragonMonitor creates a TetragonMonitor with the given config and connector.
// If connector is nil, the monitor runs in simulation mode (no real gRPC connection).
func NewTetragonMonitor(cfg TetragonConfig, connector TetragonConnector) *TetragonMonitor {
	return &TetragonMonitor{
		config:          cfg,
		connector:       connector,
		logger:          log.New(os.Stderr, "[ebeecontrol:tetragon] ", log.LstdFlags|log.Lmsgprefix),
		registeredPaths: make(map[string]struct{}),
		buffer:          NewEventBuffer(cfg.EventBufferCapacity),
		done:            make(chan struct{}),
	}
}

// Start begins listening for Tetragon events. Non-blocking; spawns a goroutine.
func (m *TetragonMonitor) Start(ctx context.Context) error {
	if m.connector == nil {
		m.mu.Lock()
		m.connected = true
		m.mu.Unlock()
		m.logger.Println("running in simulation mode (no gRPC connector)")
		return nil
	}

	streamCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel

	go m.runStreamLoop(streamCtx)
	return nil
}

// Stop disconnects from Tetragon and stops the event loop.
func (m *TetragonMonitor) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
	m.mu.Lock()
	m.connected = false
	m.mu.Unlock()
}

// IsConnected returns whether the monitor has an active Tetragon connection.
func (m *TetragonMonitor) IsConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connected
}

// RegisterPath adds a honeytoken file path to the set of monitored paths.
// Only events matching registered paths are forwarded to callbacks.
func (m *TetragonMonitor) RegisterPath(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.registeredPaths[path] = struct{}{}
}

// UnregisterPath removes a honeytoken file path from the monitored set.
func (m *TetragonMonitor) UnregisterPath(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.registeredPaths, path)
}

// RegisteredPaths returns the current set of monitored paths.
func (m *TetragonMonitor) RegisteredPaths() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	paths := make([]string, 0, len(m.registeredPaths))
	for p := range m.registeredPaths {
		paths = append(paths, p)
	}
	return paths
}

// OnEvent registers a callback invoked for every AccessEvent detected.
func (m *TetragonMonitor) OnEvent(callback func(AccessEvent)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callbacks = append(m.callbacks, callback)
}

// BufferStatus returns the current state of the event buffer.
func (m *TetragonMonitor) BufferStatus() EventBufferStatus {
	return m.buffer.Status()
}

// InjectEvent allows manual injection of a simulated event (for testing/simulation).
func (m *TetragonMonitor) InjectEvent(event TetragonKprobeEvent) {
	m.handleRawEvent(event)
}

// runStreamLoop connects to Tetragon and processes events, reconnecting on failure.
func (m *TetragonMonitor) runStreamLoop(ctx context.Context) {
	defer close(m.done)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		stream, err := m.connector(ctx, m.config.GRPCAddress)
		if err != nil {
			m.logger.Printf("connection failed: %v", err)
			m.waitReconnect(ctx)
			continue
		}

		m.mu.Lock()
		m.connected = true
		m.mu.Unlock()
		m.logger.Printf("connected to Tetragon at %s", m.config.GRPCAddress)

		m.consumeStream(ctx, stream)

		m.mu.Lock()
		m.connected = false
		m.mu.Unlock()

		_ = stream.Close()
		m.waitReconnect(ctx)
	}
}

// consumeStream reads events from the stream until error or context cancellation.
func (m *TetragonMonitor) consumeStream(ctx context.Context, stream TetragonStream) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		event, err := stream.Recv()
		if err != nil {
			m.logger.Printf("stream error: %v", err)
			return
		}
		if event != nil {
			m.handleRawEvent(*event)
		}
	}
}

// handleRawEvent transforms a Tetragon event and dispatches it if the path is registered.
func (m *TetragonMonitor) handleRawEvent(raw TetragonKprobeEvent) {
	filePath := extractFilePath(raw)

	m.mu.RLock()
	_, monitored := m.registeredPaths[filePath]
	callbacks := make([]func(AccessEvent), len(m.callbacks))
	copy(callbacks, m.callbacks)
	m.mu.RUnlock()

	if !monitored {
		return
	}

	event := TransformTetragonEvent(raw)
	m.buffer.Push(event)

	for _, cb := range callbacks {
		func() {
			defer func() {
				if r := recover(); r != nil {
					m.logger.Printf("callback panic: %v", r)
				}
			}()
			cb(event)
		}()
	}
}

// waitReconnect sleeps for the configured reconnect interval or until context is done.
func (m *TetragonMonitor) waitReconnect(ctx context.Context) {
	select {
	case <-ctx.Done():
	case <-time.After(m.config.ReconnectInterval):
	}
}

// TransformTetragonEvent converts a raw Tetragon kprobe event into an AccessEvent.
func TransformTetragonEvent(event TetragonKprobeEvent) AccessEvent {
	return AccessEvent{
		EventID:           uuid.New().String(),
		ProcessID:         event.Process.PID,
		ProcessBinaryPath: event.Process.Binary,
		UserID:            event.Process.UID,
		PodID:             event.Process.Pod.Name,
		Namespace:         event.Process.Pod.Namespace,
		HoneytokenPath:    extractFilePath(event),
		AccessType:        mapFunctionToAccessType(event.FunctionName),
		Timestamp:         event.Time,
	}
}

// mapFunctionToAccessType maps a Tetragon kprobe function name to an AccessType.
func mapFunctionToAccessType(functionName string) AccessType {
	fn := strings.ToLower(functionName)
	switch {
	case strings.Contains(fn, "fd_install") || strings.Contains(fn, "sys_open"):
		return AccessOpen
	case strings.Contains(fn, "sys_read") || strings.Contains(fn, "vfs_read"):
		return AccessRead
	case strings.Contains(fn, "sys_write") || strings.Contains(fn, "vfs_write"):
		return AccessWrite
	case strings.Contains(fn, "sys_newstat") || strings.Contains(fn, "stat"):
		return AccessStat
	default:
		return AccessRead
	}
}

// extractFilePath extracts the file path from a Tetragon kprobe event.
func extractFilePath(event TetragonKprobeEvent) string {
	if event.Args.FileArg != "" {
		return event.Args.FileArg
	}
	if event.Args.StringArg != "" {
		return event.Args.StringArg
	}
	return "unknown"
}

// --- Event Buffer ---

// EventBufferStatus reports the state of the circular event buffer.
type EventBufferStatus struct {
	Capacity int `json:"capacity"`
	Size     int `json:"size"`
	Overflow int `json:"overflow"`
}

// EventBuffer is a thread-safe circular buffer for AccessEvents with FIFO eviction.
type EventBuffer struct {
	mu       sync.Mutex
	events   []AccessEvent
	capacity int
	head     int
	size     int
	overflow int
}

// NewEventBuffer creates a circular buffer with the given capacity.
func NewEventBuffer(capacity int) *EventBuffer {
	if capacity < 1 {
		capacity = 1000
	}
	return &EventBuffer{
		events:   make([]AccessEvent, capacity),
		capacity: capacity,
	}
}

// Push adds an event to the buffer, evicting the oldest if full.
func (b *EventBuffer) Push(event AccessEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()

	idx := (b.head + b.size) % b.capacity
	if b.size == b.capacity {
		// Buffer full — overwrite oldest, advance head.
		b.events[b.head] = event
		b.head = (b.head + 1) % b.capacity
		b.overflow++
	} else {
		b.events[idx] = event
		b.size++
	}
}

// Recent returns the most recent n events (newest first).
func (b *EventBuffer) Recent(n int) []AccessEvent {
	b.mu.Lock()
	defer b.mu.Unlock()

	if n > b.size {
		n = b.size
	}
	result := make([]AccessEvent, n)
	for i := 0; i < n; i++ {
		idx := (b.head + b.size - 1 - i) % b.capacity
		result[i] = b.events[idx]
	}
	return result
}

// Status returns the current buffer utilization.
func (b *EventBuffer) Status() EventBufferStatus {
	b.mu.Lock()
	defer b.mu.Unlock()
	return EventBufferStatus{
		Capacity: b.capacity,
		Size:     b.size,
		Overflow: b.overflow,
	}
}

// String implements fmt.Stringer for EventBufferStatus.
func (s EventBufferStatus) String() string {
	return fmt.Sprintf("buffer: %d/%d (overflow: %d)", s.Size, s.Capacity, s.Overflow)
}
