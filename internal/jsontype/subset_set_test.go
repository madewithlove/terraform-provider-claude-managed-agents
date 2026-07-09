package jsontype

import (
	"context"
	"testing"
)

const (
	// config: three entries in config order, minimally specified.
	setConfig = `[{"type":"agent_toolset_20260401"},{"type":"mcp_toolset","mcp_server_name":"Team"},{"type":"mcp_toolset","mcp_server_name":"gcal-calendarmcp"}]`
	// server: same three entries, REORDERED and ENRICHED with server-added keys.
	setServer = `[{"type":"agent_toolset_20260401","default_config":{"enabled":true},"configs":[]},{"type":"mcp_toolset","mcp_server_name":"gcal-calendarmcp","default_config":{"enabled":true},"configs":[]},{"type":"mcp_toolset","mcp_server_name":"Team","default_config":{"enabled":true},"configs":[]}]`
)

func TestIsSubsetUnordered(t *testing.T) {
	tests := []struct {
		name       string
		sub, super string
		want       bool
	}{
		{"reordered and enriched multiset", setConfig, setServer, true},
		{"identical", setConfig, setConfig, true},
		{
			name:  "removed server (length differs)",
			sub:   setConfig,
			super: `[{"type":"agent_toolset_20260401","configs":[]},{"type":"mcp_toolset","mcp_server_name":"Team","configs":[]}]`,
			want:  false,
		},
		{
			name:  "added server (length differs)",
			sub:   `[{"type":"mcp_toolset","mcp_server_name":"Team"}]`,
			super: `[{"type":"mcp_toolset","mcp_server_name":"Team","configs":[]},{"type":"mcp_toolset","mcp_server_name":"Extra","configs":[]}]`,
			want:  false,
		},
		{
			name:  "changed mcp_server_name",
			sub:   `[{"type":"mcp_toolset","mcp_server_name":"Team"},{"type":"mcp_toolset","mcp_server_name":"gcal-calendarmcp"}]`,
			super: `[{"type":"mcp_toolset","mcp_server_name":"Team","configs":[]},{"type":"mcp_toolset","mcp_server_name":"Renamed","configs":[]}]`,
			want:  false,
		},
		{
			name:  "changed nested value within a matched element",
			sub:   `[{"type":"mcp_toolset","mcp_server_name":"Team","allowed_tools":["a"]}]`,
			super: `[{"type":"mcp_toolset","mcp_server_name":"Team","allowed_tools":["b"],"configs":[]}]`,
			want:  false,
		},
		{
			// Two identical config entries claim two distinct server entries.
			name:  "duplicate identical entries match distinct servers",
			sub:   `[{"type":"t"},{"type":"t"}]`,
			super: `[{"type":"t"},{"type":"t"}]`,
			want:  true,
		},
		{
			name:  "duplicate config cannot both match one enriched server",
			sub:   `[{"type":"t","k":1},{"type":"t","k":1}]`,
			super: `[{"type":"t","k":1,"x":9},{"type":"t","k":2}]`,
			want:  false, // second config (k=1) has no unclaimed server with type=t AND k=1
		},
		{
			name:  "single mcp server reordered no-op",
			sub:   `[{"type":"mcp_toolset","mcp_server_name":"Team"}]`,
			super: `[{"type":"mcp_toolset","mcp_server_name":"Team","configs":[],"default_config":{"enabled":true}}]`,
			want:  true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := IsSubsetUnordered([]byte(tc.sub), []byte(tc.super))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("IsSubsetUnordered(%s, %s) = %v, want %v", tc.sub, tc.super, got, tc.want)
			}
		})
	}
}

// TestSubsetSetSemanticEquals_RefreshDirection pins the refresh call direction
// for the set type: proposedNew(server).StringSemanticEquals(prior config) is
// true when the reordered+enriched server value contains the same set.
func TestSubsetSetSemanticEquals_RefreshDirection(t *testing.T) {
	ctx := context.Background()
	server := NewSubsetSetValue(setServer)
	config := NewSubsetSetValue(setConfig)

	if keep, _ := server.StringSemanticEquals(ctx, config); !keep {
		t.Fatal("expected server.StringSemanticEquals(config) == true for reordered+enriched multiset")
	}
	// A genuine removal must not be equal in either direction.
	removed := NewSubsetSetValue(`[{"type":"mcp_toolset","mcp_server_name":"Team"}]`)
	if keep, _ := server.StringSemanticEquals(ctx, removed); keep {
		t.Fatal("expected not-equal when the prior set is smaller than the server set")
	}
}
