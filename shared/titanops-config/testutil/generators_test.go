package testutil_test

import (
	"testing"

	"github.com/mercadoalex/titanops/shared/titanops-config/testutil"
	"pgregory.net/rapid"
)

func TestValidEventGenerator(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		ev := testutil.ValidEvent().Draw(t, "event")
		if ev.Namespace == "" {
			t.Fatal("expected non-empty namespace")
		}
		if ev.Severity == "" {
			t.Fatal("expected non-empty severity")
		}
		if ev.Module == "" {
			t.Fatal("expected non-empty module")
		}
		if ev.EventType == "" {
			t.Fatal("expected non-empty event type")
		}
		if len(ev.Payload) == 0 {
			t.Fatal("expected non-empty payload")
		}
		if len(ev.Payload) > 65536 {
			t.Fatalf("payload too large: %d bytes", len(ev.Payload))
		}
	})
}

func TestFeatureVectorGenerator(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		vec := testutil.FeatureVectorVariable(1, 128).Draw(t, "vec")
		if len(vec) < 1 || len(vec) > 128 {
			t.Fatalf("unexpected vector length: %d", len(vec))
		}
		for i, v := range vec {
			if v < 0 || v > 1 {
				t.Fatalf("feature[%d] out of [0,1] range: %f", i, v)
			}
		}
	})
}

func TestValidPortGenerator(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		port := testutil.ValidPort().Draw(t, "port")
		if port < 1024 || port > 65535 {
			t.Fatalf("port %d out of valid range [1024, 65535]", port)
		}
	})
}

func TestInvalidPortGenerator(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		port := testutil.InvalidPort().Draw(t, "port")
		if port >= 1024 && port <= 65535 {
			t.Fatalf("expected invalid port, got valid: %d", port)
		}
	})
}

func TestSeverityGenerator(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		sev := testutil.Severity().Draw(t, "severity")
		valid := false
		for _, s := range testutil.Severities {
			if sev == s {
				valid = true
				break
			}
		}
		if !valid {
			t.Fatalf("invalid severity: %s", sev)
		}
	})
}

func TestModuleGenerator(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		mod := testutil.Module().Draw(t, "module")
		valid := false
		for _, m := range testutil.Modules {
			if mod == m {
				valid = true
				break
			}
		}
		if !valid {
			t.Fatalf("invalid module: %s", mod)
		}
	})
}

func TestConfidenceScoreGenerator(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		score := testutil.ConfidenceScore().Draw(t, "score")
		if score < 0.0 || score > 1.0 {
			t.Fatalf("confidence score %f out of [0.0, 1.0] range", score)
		}
	})
}

func TestValidExportConfigGenerator(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cfg := testutil.ValidExportConfig().Draw(t, "config")
		if cfg.PrometheusPort < 1024 || cfg.PrometheusPort > 65535 {
			t.Fatalf("prometheus port %d out of valid range", cfg.PrometheusPort)
		}
	})
}
