package jsontype

import (
	"context"
	"testing"
)

const (
	// config-side (what the user writes)
	toolsConfig = `[{"type":"agent_toolset_20260401"}]`
	// server-side (enriched on the way back)
	toolsServer = `[{"type":"agent_toolset_20260401","default_config":{"enabled":true,"permission_policy":{"type":"always_allow"}}}]`
)

func TestIsSubset(t *testing.T) {
	tests := []struct {
		name       string
		sub, super string
		want       bool
	}{
		{"enrichment tolerated", toolsConfig, toolsServer, true},
		{"identical", toolsConfig, toolsConfig, true},
		{"superset is not a subset of subset", toolsServer, toolsConfig, false},
		{"changed scalar", `[{"type":"a"}]`, `[{"type":"b","default_config":{}}]`, false},
		{"removed array element (length differs)", `[{"type":"a"}]`, `[{"type":"a"},{"type":"b"}]`, false},
		{"added array element (length differs)", `[{"type":"a"},{"type":"b"}]`, `[{"type":"a"}]`, false},
		{"nested object subset", `{"a":{"x":1}}`, `{"a":{"x":1,"y":2},"z":3}`, true},
		{"nested object missing key", `{"a":{"x":1,"q":9}}`, `{"a":{"x":1,"y":2}}`, false},
		{"number preserved", `{"n":10000000000000000001}`, `{"n":10000000000000000001,"extra":1}`, true},
		{"bool mismatch", `{"b":true}`, `{"b":false}`, false},
		{"null match", `{"a":null}`, `{"a":null,"b":1}`, true},
		{"empty object subset of anything object", `{}`, `{"a":1}`, true},
		{"object vs array mismatch", `{"a":1}`, `[1,2]`, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := IsSubset([]byte(tc.sub), []byte(tc.super))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("IsSubset(%s, %s) = %v, want %v", tc.sub, tc.super, got, tc.want)
			}
		})
	}
}

// TestStringSemanticEquals_RefreshDirection pins the exact call direction the
// framework uses during a refresh:
//
//	proposedNew.StringSemanticEquals(ctx, prior)
//
// where proposedNew is the freshly-read (enriched, server) value and prior is
// the state value (the user's config-shaped subset). Returning true keeps the
// prior value, avoiding a diff. This is the direction that matters for a clean
// plan; get it backwards and either every refresh churns or unrelated values
// are silently treated as equal.
func TestStringSemanticEquals_RefreshDirection(t *testing.T) {
	ctx := context.Background()
	server := NewSubsetValue(toolsServer)
	config := NewSubsetValue(toolsConfig)

	// Refresh direction: receiver = server (proposed new), arg = config (prior).
	keepPrior, diags := server.StringSemanticEquals(ctx, config)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if !keepPrior {
		t.Fatal("expected server.StringSemanticEquals(config) == true (prior config is a subset of enriched server value; keep prior)")
	}

	// Reversed direction must NOT be equal — the server value is not a subset
	// of the config value, so we must not silently treat them as equal.
	if reversed, _ := config.StringSemanticEquals(ctx, server); reversed {
		t.Fatal("expected config.StringSemanticEquals(server) == false")
	}
}

func TestStringSemanticEquals_GenuineChange(t *testing.T) {
	ctx := context.Background()
	server := NewSubsetValue(`[{"type":"agent_toolset_20260401","default_config":{}}]`)
	changed := NewSubsetValue(`[{"type":"something_else"}]`)

	// A genuine change (prior no longer a subset of the new value) is not equal.
	if eq, _ := server.StringSemanticEquals(ctx, changed); eq {
		t.Fatal("expected a genuinely changed prior to compare not-equal")
	}
}

func TestStringSemanticEquals_NullHandling(t *testing.T) {
	ctx := context.Background()
	server := NewSubsetValue(toolsServer)
	null := NewSubsetNull()

	// New value present, prior null (the import case): not equal, so the new
	// (enriched) value is adopted into state rather than keeping null.
	if eq, _ := server.StringSemanticEquals(ctx, null); eq {
		t.Fatal("expected present-vs-null to compare not-equal")
	}
	// Both null: equal.
	if eq, _ := null.StringSemanticEquals(ctx, NewSubsetNull()); !eq {
		t.Fatal("expected null-vs-null to compare equal")
	}
}
