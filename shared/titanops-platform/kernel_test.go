package platform

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	export "github.com/mercadoalex/titanops/shared/titanops-export"
)

// --- Mock Module ---

type mockModule struct {
	id         string
	version    string
	startErr   error
	stopErr    error
	health     HealthStatus
	startCalls int32
	stopCalls  int32
	shouldPanic bool
}

func (m *mockModule) ID() string      { return m.id }
func (m *mockModule) Version() string  { return m.version }

func (m *mockModule) Start(_ context.Context, _ Kernel) error {
	atomic.AddInt32(&m.startCalls, 1)
	if m.shouldPanic {
		panic("test panic")
	}
	return m.startErr
}

func (m *mockModule) Stop(_ context.Context) error {
	atomic.AddInt32(&m.stopCalls, 1)
	return m.stopErr
}

func (m *mockModule) HealthCheck(_ context.Context) HealthStatus {
	return m.health
}

// --- Mock Emitter ---

type mockEmitter struct {
	events []export.Event
}

func (e *mockEmitter) Emit(_ context.Context, event export.Event) error {
	e.events = append(e.events, event)
	return nil
}

// --- Tests ---

func TestKernel_RegisterAndStartModule(t *testing.T) {
	kernel := NewKernel("test-cluster")
	emitter := &mockEmitter{}
	kernel.SetEmitter(emitter)

	mod := &mockModule{
		id:      "test-module",
		version: "v1.0.0",
		health:  HealthStatus{State: HealthHealthy, Message: "ok", LastCheck: time.Now()},
	}

	if err := kernel.Register(mod); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	errs := kernel.StartAll(context.Background())
	if len(errs) != 0 {
		t.Fatalf("start failed: %v", errs)
	}

	if atomic.LoadInt32(&mod.startCalls) != 1 {
		t.Fatal("module Start not called")
	}
}

func TestKernel_DuplicateRegisterFails(t *testing.T) {
	kernel := NewKernel("test-cluster")

	mod := &mockModule{id: "dup", version: "v1.0.0"}
	if err := kernel.Register(mod); err != nil {
		t.Fatalf("first register failed: %v", err)
	}

	if err := kernel.Register(mod); err == nil {
		t.Fatal("duplicate register should fail")
	}
}

func TestKernel_ModuleStartFailureDoesNotCrashPlatform(t *testing.T) {
	kernel := NewKernel("test-cluster")
	kernel.SetEmitter(&mockEmitter{})

	good := &mockModule{id: "good", version: "v1.0.0", health: HealthStatus{State: HealthHealthy}}
	bad := &mockModule{id: "bad", version: "v1.0.0", startErr: errors.New("broken")}

	kernel.Register(good)
	kernel.Register(bad)

	errs := kernel.StartAll(context.Background())

	// One error expected (from "bad"), but "good" should still start.
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}

	if atomic.LoadInt32(&good.startCalls) != 1 {
		t.Fatal("good module should have started")
	}
}

func TestKernel_PanicRecoveryOnStart(t *testing.T) {
	kernel := NewKernel("test-cluster")
	kernel.SetEmitter(&mockEmitter{})

	panicker := &mockModule{id: "panicker", version: "v1.0.0", shouldPanic: true}
	healthy := &mockModule{id: "healthy", version: "v1.0.0", health: HealthStatus{State: HealthHealthy}}

	kernel.Register(panicker)
	kernel.Register(healthy)

	errs := kernel.StartAll(context.Background())

	// Panicker should produce an error, but healthy should start fine.
	if len(errs) != 1 {
		t.Fatalf("expected 1 error from panic, got %d: %v", len(errs), errs)
	}

	if atomic.LoadInt32(&healthy.startCalls) != 1 {
		t.Fatal("healthy module should have started despite panicker")
	}
}

func TestKernel_StopAll(t *testing.T) {
	kernel := NewKernel("test-cluster")
	kernel.SetEmitter(&mockEmitter{})

	mod := &mockModule{id: "stoppable", version: "v1.0.0", health: HealthStatus{State: HealthHealthy}}
	kernel.Register(mod)
	kernel.StartAll(context.Background())

	kernel.StopAll(context.Background())

	if atomic.LoadInt32(&mod.stopCalls) != 1 {
		t.Fatal("module Stop not called")
	}
}

func TestKernel_HealthCheckAll(t *testing.T) {
	kernel := NewKernel("test-cluster")
	kernel.SetEmitter(&mockEmitter{})

	mod1 := &mockModule{id: "mod1", version: "v1.0.0", health: HealthStatus{State: HealthHealthy, Message: "all good"}}
	mod2 := &mockModule{id: "mod2", version: "v1.0.0", health: HealthStatus{State: HealthDegraded, Message: "partial"}}

	kernel.Register(mod1)
	kernel.Register(mod2)
	kernel.StartAll(context.Background())

	results := kernel.HealthCheckAll(context.Background())

	if len(results) != 2 {
		t.Fatalf("expected 2 health results, got %d", len(results))
	}

	if results["mod1"].State != HealthHealthy {
		t.Fatalf("mod1 should be healthy, got %q", results["mod1"].State)
	}
	if results["mod2"].State != HealthDegraded {
		t.Fatalf("mod2 should be degraded, got %q", results["mod2"].State)
	}
}

func TestKernel_EmitRoutesToSubscribers(t *testing.T) {
	kernel := NewKernel("test-cluster")
	emitter := &mockEmitter{}
	kernel.SetEmitter(emitter)

	ch := make(chan export.Event, 10)
	kernel.Subscribe(EventFilter{Module: "ebeecontrol"}, func(_ context.Context, event export.Event) {
		ch <- event
	})

	event := export.Event{
		Module:    "ebeecontrol",
		EventType: "honeytoken_access",
		Severity:  "high",
		EventID:   "evt-1",
		Timestamp: time.Now(),
	}

	err := kernel.Emit(context.Background(), event)
	if err != nil {
		t.Fatalf("emit failed: %v", err)
	}

	select {
	case received := <-ch:
		if received.EventID != "evt-1" {
			t.Fatalf("wrong event delivered: %s", received.EventID)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for event")
	}

	// Emitter should also have received it.
	if len(emitter.events) != 1 {
		t.Fatalf("emitter should have 1 event, got %d", len(emitter.events))
	}
}

func TestKernel_SubscribeFiltersByModule(t *testing.T) {
	kernel := NewKernel("test-cluster")
	kernel.SetEmitter(&mockEmitter{})

	ch := make(chan export.Event, 10)
	kernel.Subscribe(EventFilter{Module: "earthworm"}, func(_ context.Context, event export.Event) {
		ch <- event
	})

	// Emit an ebeecontrol event — should NOT be delivered.
	kernel.Emit(context.Background(), export.Event{Module: "ebeecontrol", EventID: "skip"})
	// Emit an earthworm event — should be delivered.
	kernel.Emit(context.Background(), export.Event{Module: "earthworm", EventID: "match"})

	select {
	case received := <-ch:
		if received.EventID != "match" {
			t.Fatalf("wrong event passed filter: %s", received.EventID)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for filtered event")
	}

	// Verify the skipped event didn't arrive.
	select {
	case extra := <-ch:
		t.Fatalf("unexpected extra event: %s", extra.EventID)
	case <-time.After(100 * time.Millisecond):
		// Good — no extra events.
	}
}

func TestKernel_GetModuleReturnsNilForUnknown(t *testing.T) {
	kernel := NewKernel("test-cluster")

	if m := kernel.GetModule("nonexistent"); m != nil {
		t.Fatal("expected nil for unknown module")
	}
}

func TestKernel_GetModuleReturnsRunningModule(t *testing.T) {
	kernel := NewKernel("test-cluster")
	kernel.SetEmitter(&mockEmitter{})

	mod := &mockModule{id: "findme", version: "v1.0.0"}
	kernel.Register(mod)
	kernel.StartAll(context.Background())

	found := kernel.GetModule("findme")
	if found == nil {
		t.Fatal("expected to find running module")
	}
	if found.ID() != "findme" {
		t.Fatalf("wrong module returned: %s", found.ID())
	}
}

func TestKernel_ClusterID(t *testing.T) {
	kernel := NewKernel("my-cluster-42")
	if kernel.ClusterID() != "my-cluster-42" {
		t.Fatalf("expected 'my-cluster-42', got %q", kernel.ClusterID())
	}
}
