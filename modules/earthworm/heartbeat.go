package earthworm

import "time"

// HeartbeatSignal represents a cluster node health signal
// collected from eBPF probes for anomaly analysis.
type HeartbeatSignal struct {
	// NodeID is the Kubernetes node identifier.
	NodeID string
	// CPUUsage is the CPU utilization percentage [0.0, 1.0].
	CPUUsage float64
	// MemoryUsage is the memory utilization percentage [0.0, 1.0].
	MemoryUsage float64
	// DiskIO is the disk I/O utilization percentage [0.0, 1.0].
	DiskIO float64
	// NetworkIO is the network I/O utilization percentage [0.0, 1.0].
	NetworkIO float64
	// Latency is the average response latency in milliseconds.
	Latency float64
	// Timestamp is when the heartbeat was collected.
	Timestamp time.Time
}

// ToFeatures converts the heartbeat signal into a feature vector
// suitable for ONNX model inference. The vector contains metrics
// in a fixed order: CPU, Memory, DiskIO, NetworkIO, Latency (normalized).
func (h HeartbeatSignal) ToFeatures() []float32 {
	// Normalize latency to [0, 1] range assuming max 1000ms.
	normalizedLatency := h.Latency / 1000.0
	if normalizedLatency > 1.0 {
		normalizedLatency = 1.0
	}
	if normalizedLatency < 0.0 {
		normalizedLatency = 0.0
	}

	return []float32{
		float32(h.CPUUsage),
		float32(h.MemoryUsage),
		float32(h.DiskIO),
		float32(h.NetworkIO),
		float32(normalizedLatency),
	}
}
