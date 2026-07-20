package export

import (
	"bytes"
	"encoding/json"
	"sync"
)

// bufPool provides reusable byte buffers for event serialization.
// Inspired by VictoriaMetrics' zero-allocation patterns — under sustained
// event throughput, buffer reuse prevents repeated allocations and reduces
// GC pressure on the hot path.
//
// Each buffer starts at 4KB and grows as needed; the pool caps at 64KB
// per buffer to avoid holding oversized buffers indefinitely.
var bufPool = sync.Pool{
	New: func() any {
		return bytes.NewBuffer(make([]byte, 0, 4096))
	},
}

// maxPooledBufSize is the maximum buffer size we return to the pool.
// Buffers that grew beyond this (e.g., due to a large payload) are
// discarded to GC instead of polluting the pool with oversized buffers.
const maxPooledBufSize = 65536

// MarshalEvent serializes an Event to JSON using a pooled buffer.
// The returned byte slice is a copy safe for the caller to own.
// This avoids per-call allocations under sustained throughput.
func MarshalEvent(event Event) ([]byte, error) {
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()

	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(event); err != nil {
		putBuf(buf)
		return nil, err
	}

	// Encode adds a trailing newline; trim it for clean JSON.
	data := buf.Bytes()
	if len(data) > 0 && data[len(data)-1] == '\n' {
		data = data[:len(data)-1]
	}

	// Copy out so the caller owns the slice (buffer goes back to pool).
	result := make([]byte, len(data))
	copy(result, data)

	putBuf(buf)
	return result, nil
}

// MarshalEventTo serializes an Event into the provided buffer, resizing if needed.
// Returns the serialized bytes (may be a sub-slice of dst or a new allocation
// if dst was too small). This variant avoids even the final copy when the caller
// can manage buffer lifecycle (e.g., publish then release).
func MarshalEventTo(event Event, dst []byte) ([]byte, error) {
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()

	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(event); err != nil {
		putBuf(buf)
		return nil, err
	}

	data := buf.Bytes()
	if len(data) > 0 && data[len(data)-1] == '\n' {
		data = data[:len(data)-1]
	}

	// Try to fit into caller's buffer
	if cap(dst) >= len(data) {
		dst = dst[:len(data)]
		copy(dst, data)
		putBuf(buf)
		return dst, nil
	}

	// Caller's buffer too small — allocate
	result := make([]byte, len(data))
	copy(result, data)
	putBuf(buf)
	return result, nil
}

// putBuf returns a buffer to the pool if it hasn't grown too large.
func putBuf(buf *bytes.Buffer) {
	if buf.Cap() <= maxPooledBufSize {
		bufPool.Put(buf)
	}
	// Oversized buffers are silently dropped — GC will reclaim them.
}
