package correlation

import (
	"fmt"
	"time"

	export "github.com/mercadoalex/titanops/shared/titanops-export"
)

// dedupEntry tracks when a specific event fingerprint was last seen.
type dedupEntry struct {
	lastSeen time.Time
}

// dedupCache provides event deduplication within a configurable time window.
// Events with the same fingerprint (module + event_type + node) arriving within
// the dedup window are collapsed — only the first is kept.
//
// Inspired by VictoriaMetrics' deduplication during merge, adapted for the
// streaming event correlation use case.
type dedupCache struct {
	window  time.Duration
	entries map[string]dedupEntry
}

// newDedupCache creates a dedup cache with the given window duration.
// If window is zero, dedup is disabled and IsDuplicate always returns false.
func newDedupCache(window time.Duration) *dedupCache {
	return &dedupCache{
		window:  window,
		entries: make(map[string]dedupEntry),
	}
}

// IsDuplicate checks whether the event is a duplicate within the dedup window.
// If it is NOT a duplicate, it records the event and returns false.
// If it IS a duplicate, it returns true (caller should skip the event).
//
// Must be called under the engine's write lock.
func (d *dedupCache) IsDuplicate(event export.Event, now time.Time) bool {
	if d.window <= 0 {
		return false
	}

	key := eventFingerprint(event)
	entry, exists := d.entries[key]

	if exists && now.Sub(entry.lastSeen) < d.window {
		// Duplicate — same fingerprint within the window
		return true
	}

	// Not a duplicate — record it
	d.entries[key] = dedupEntry{lastSeen: now}
	return false
}

// Prune removes entries older than the dedup window.
// Should be called periodically to prevent unbounded memory growth.
func (d *dedupCache) Prune(now time.Time) {
	if d.window <= 0 {
		return
	}
	cutoff := now.Add(-d.window)
	for key, entry := range d.entries {
		if entry.lastSeen.Before(cutoff) {
			delete(d.entries, key)
		}
	}
}

// Len returns the number of entries in the dedup cache.
func (d *dedupCache) Len() int {
	return len(d.entries)
}

// eventFingerprint produces a dedup key from an event.
// Two events are considered duplicates if they share the same module,
// event type, and node within the dedup window.
func eventFingerprint(event export.Event) string {
	return fmt.Sprintf("%s|%s|%s", event.Module, event.EventType, event.Node)
}
