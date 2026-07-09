package provider

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/madewithlove/terraform-provider-claude-managed-agents/internal/jsontype"
)

// pathRoot is a small alias to keep provider.go readable.
func pathRoot(name string) path.Path { return path.Root(name) }

// stringPtr returns a *string for a framework String, or nil when null/unknown.
func stringPtr(s types.String) *string {
	if s.IsNull() || s.IsUnknown() {
		return nil
	}
	v := s.ValueString()
	return &v
}

// stringFromPtr maps a *string to a framework String (null when nil).
func stringFromPtr(p *string) types.String {
	if p == nil {
		return types.StringNull()
	}
	return types.StringValue(*p)
}

// rawFromNormalized converts a jsontypes.Normalized to json.RawMessage,
// returning nil when the value is null or unknown.
func rawFromNormalized(n jsontypes.Normalized) json.RawMessage {
	if n.IsNull() || n.IsUnknown() {
		return nil
	}
	return json.RawMessage(n.ValueString())
}

// normalizedFromRaw wraps raw JSON in a jsontypes.Normalized, mapping an empty
// or JSON-null payload to a null value.
func normalizedFromRaw(raw json.RawMessage) jsontypes.Normalized {
	if len(raw) == 0 || string(raw) == "null" {
		return jsontypes.NewNormalizedNull()
	}
	return jsontypes.NewNormalizedValue(string(raw))
}

// rawFromSubset converts a jsontype.Subset to json.RawMessage, returning nil
// when the value is null or unknown.
func rawFromSubset(n jsontype.Subset) json.RawMessage {
	if n.IsNull() || n.IsUnknown() {
		return nil
	}
	return json.RawMessage(n.ValueString())
}

// subsetFromRaw wraps raw JSON in a jsontype.Subset for an object-shaped field
// (e.g. multiagent). An empty, JSON-null, or empty payload maps to null: the
// coordinator is simply absent, which config also expresses by omission, so
// null-vs-null never diffs.
func subsetFromRaw(raw json.RawMessage) jsontype.Subset {
	if isEmptyJSON(raw) {
		return jsontype.NewSubsetNull()
	}
	return jsontype.NewSubsetValue(string(raw))
}

// subsetFromRawArray wraps raw JSON in a jsontype.Subset for an array-shaped
// field (e.g. skills). An empty/absent value canonicalizes to `[]` rather than
// null so it matches a config that writes `jsonencode([])`; a config that omits
// the field (null) is carried forward from this state by UseStateForUnknown, so
// both spellings of "no entries" plan cleanly. Terraform forbids planning null
// against a non-null config value, which is why we canonicalize to `[]` rather
// than null here.
func subsetFromRawArray(raw json.RawMessage) jsontype.Subset {
	if isEmptyJSON(raw) {
		return jsontype.NewSubsetValue("[]")
	}
	return jsontype.NewSubsetValue(string(raw))
}

// rawFromSubsetSet converts a jsontype.SubsetSet to json.RawMessage, returning
// nil when the value is null or unknown.
func rawFromSubsetSet(n jsontype.SubsetSet) json.RawMessage {
	if n.IsNull() || n.IsUnknown() {
		return nil
	}
	return json.RawMessage(n.ValueString())
}

// subsetSetFromRaw is subsetFromRawArray for the order-insensitive SubsetSet
// type (tools, mcp_servers): empty/absent canonicalizes to `[]`.
func subsetSetFromRaw(raw json.RawMessage) jsontype.SubsetSet {
	if isEmptyJSON(raw) {
		return jsontype.NewSubsetSetValue("[]")
	}
	return jsontype.NewSubsetSetValue(string(raw))
}

func isEmptyJSON(raw json.RawMessage) bool {
	switch strings.TrimSpace(string(raw)) {
	case "", "null", "[]", "{}":
		return true
	}
	return false
}

// listToStrings converts a framework List of strings to a Go slice, returning
// nil when null or unknown.
func listToStrings(ctx context.Context, l types.List) ([]string, diag.Diagnostics) {
	if l.IsNull() || l.IsUnknown() {
		return nil, nil
	}
	out := make([]string, 0, len(l.Elements()))
	diags := l.ElementsAs(ctx, &out, false)
	return out, diags
}

// stringsToList converts a Go slice to a framework List, mapping an empty
// slice to a null list.
func stringsToList(ctx context.Context, s []string) (types.List, diag.Diagnostics) {
	if len(s) == 0 {
		return types.ListNull(types.StringType), nil
	}
	return types.ListValueFrom(ctx, types.StringType, s)
}

// mapToStringMap converts a framework Map to a Go map, returning nil when the
// value is null or unknown.
func mapToStringMap(ctx context.Context, m types.Map) (map[string]string, diag.Diagnostics) {
	if m.IsNull() || m.IsUnknown() {
		return nil, nil
	}
	out := make(map[string]string, len(m.Elements()))
	diags := m.ElementsAs(ctx, &out, false)
	return out, diags
}

// stringMapToMap converts a Go map to a framework Map.
func stringMapToMap(ctx context.Context, m map[string]string) (types.Map, diag.Diagnostics) {
	return types.MapValueFrom(ctx, types.StringType, m)
}

// mergedMetadata builds the metadata payload for an update. Keys present in the
// plan are set to their new value; keys that were in state but dropped from the
// plan are set to null so the API deletes them (the API merges metadata by
// key). Returns nil when there is nothing to send.
func mergedMetadata(ctx context.Context, state, plan types.Map) (map[string]interface{}, diag.Diagnostics) {
	var diags diag.Diagnostics

	planMap, d := mapToStringMap(ctx, plan)
	diags.Append(d...)
	stateMap, d := mapToStringMap(ctx, state)
	diags.Append(d...)
	if diags.HasError() {
		return nil, diags
	}

	out := make(map[string]interface{}, len(planMap)+len(stateMap))
	for k, v := range planMap {
		out[k] = v
	}
	for k := range stateMap {
		if _, ok := planMap[k]; !ok {
			out[k] = nil // signal deletion
		}
	}
	if len(out) == 0 {
		return nil, diags
	}
	return out, diags
}
