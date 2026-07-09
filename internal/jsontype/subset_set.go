package jsontype

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/attr/xattr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// SubsetSet is like Subset, but treats the top-level JSON array as an unordered
// multiset: the config value is equal to the server value when the two arrays
// have the same length and there is a bijection matching every config element
// to a distinct server element under the recursive subset rule. This suits
// fields such as an agent's `tools` / `mcp_servers`, where the API returns the
// entries in an arbitrary order (and enriches each) but the set is what matters.
//
// Ordering below the top level stays significant (nested arrays are compared
// positionally by the recursive subset rule).

// ---- Type ----

var _ basetypes.StringTypable = (*SubsetSetType)(nil)

// SubsetSetType is the attribute type for SubsetSet values.
type SubsetSetType struct {
	SubsetType
}

func (t SubsetSetType) String() string { return "jsontype.SubsetSetType" }

func (t SubsetSetType) ValueType(_ context.Context) attr.Value { return SubsetSet{} }

func (t SubsetSetType) Equal(o attr.Type) bool {
	other, ok := o.(SubsetSetType)
	if !ok {
		return false
	}
	return t.SubsetType.Equal(other.SubsetType)
}

func (t SubsetSetType) ValueFromString(_ context.Context, in basetypes.StringValue) (basetypes.StringValuable, diag.Diagnostics) {
	return SubsetSet{StringValue: in}, nil
}

func (t SubsetSetType) ValueFromTerraform(ctx context.Context, in tftypes.Value) (attr.Value, error) {
	attrValue, err := t.SubsetType.NormalizedType.StringType.ValueFromTerraform(ctx, in)
	if err != nil {
		return nil, err
	}
	stringValue, ok := attrValue.(basetypes.StringValue)
	if !ok {
		return nil, fmt.Errorf("unexpected value type of %T", attrValue)
	}
	stringValuable, diags := t.ValueFromString(ctx, stringValue)
	if diags.HasError() {
		return nil, fmt.Errorf("unexpected error converting StringValue to StringValuable: %v", diags)
	}
	return stringValuable, nil
}

// ---- Value ----

var (
	_ basetypes.StringValuable                   = (*SubsetSet)(nil)
	_ basetypes.StringValuableWithSemanticEquals = (*SubsetSet)(nil)
	_ xattr.ValidateableAttribute                = (*SubsetSet)(nil)
)

// SubsetSet is a JSON string value whose semantic equality is enrichment- and
// order-tolerant for the top-level array.
type SubsetSet struct {
	basetypes.StringValue
}

func (v SubsetSet) Type(_ context.Context) attr.Type { return SubsetSetType{} }

func (v SubsetSet) Equal(o attr.Value) bool {
	other, ok := o.(SubsetSet)
	if !ok {
		return false
	}
	return v.StringValue.Equal(other.StringValue)
}

// StringSemanticEquals keeps the prior (config-shaped) value when it is an
// unordered subset of the new (enriched, possibly reordered) server value. See
// Subset.StringSemanticEquals for the call-direction contract.
func (v SubsetSet) StringSemanticEquals(_ context.Context, priorValuable basetypes.StringValuable) (bool, diag.Diagnostics) {
	var diags diag.Diagnostics

	prior, ok := priorValuable.(SubsetSet)
	if !ok {
		diags.AddError(
			"Semantic Equality Check Error",
			"An unexpected value type was received while performing semantic equality checks. "+
				"Please report this to the provider developers.\n\n"+
				"Expected Value Type: "+fmt.Sprintf("%T", v)+"\n"+
				"Got Value Type: "+fmt.Sprintf("%T", priorValuable),
		)
		return false, diags
	}

	if v.IsNull() || v.IsUnknown() || prior.IsNull() || prior.IsUnknown() {
		return v.StringValue.Equal(prior.StringValue), diags
	}

	equal, err := IsSubsetUnordered([]byte(prior.ValueString()), []byte(v.ValueString()))
	if err != nil {
		diags.AddError(
			"Semantic Equality Check Error",
			"An unexpected error occurred while performing semantic equality checks. "+
				"Please report this to the provider developers.\n\nError: "+err.Error(),
		)
		return false, diags
	}
	return equal, diags
}

// ValidateAttribute requires the value to be valid JSON.
func (v SubsetSet) ValidateAttribute(_ context.Context, req xattr.ValidateAttributeRequest, resp *xattr.ValidateAttributeResponse) {
	if v.IsUnknown() || v.IsNull() {
		return
	}
	if !json.Valid([]byte(v.ValueString())) {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid JSON String Value",
			"A string value was provided that is not valid JSON string format (RFC 7159).",
		)
	}
}

// NewSubsetSetValue returns a known SubsetSet value.
func NewSubsetSetValue(value string) SubsetSet {
	return SubsetSet{StringValue: basetypes.NewStringValue(value)}
}

// NewSubsetSetNull returns a null SubsetSet value.
func NewSubsetSetNull() SubsetSet {
	return SubsetSet{StringValue: basetypes.NewStringNull()}
}

// ---- Unordered subset comparison ----

// IsSubsetUnordered reports whether the top-level JSON array sub is an unordered
// multiset subset of the top-level JSON array super: same length, and every
// element of sub matches a distinct element of super under the recursive
// (order-preserving) subset rule. When either side is not a top-level array,
// it falls back to the ordered IsSubset semantics.
func IsSubsetUnordered(sub, super []byte) (bool, error) {
	subv, err := decodeJSON(sub)
	if err != nil {
		return false, err
	}
	superv, err := decodeJSON(super)
	if err != nil {
		return false, err
	}
	return valueIsSubsetUnordered(subv, superv), nil
}

func valueIsSubsetUnordered(sub, super interface{}) bool {
	subArr, ok1 := sub.([]interface{})
	superArr, ok2 := super.([]interface{})
	if !ok1 || !ok2 {
		// Not both top-level arrays: use ordered semantics.
		return valueIsSubset(sub, super)
	}
	if len(subArr) != len(superArr) {
		return false // a genuine add or remove
	}

	claimed := make([]bool, len(superArr))
	for _, se := range subArr {
		idx := matchServerElement(se, superArr, claimed)
		if idx < 0 {
			return false
		}
		claimed[idx] = true
	}
	// Equal lengths + every config element claimed a distinct server element
	// means the bijection is complete with no leftovers.
	return true
}

// matchServerElement returns the index of an unclaimed server element that the
// config element matches, or -1. When the config element has a natural key
// (mcp_server_name, else type), only server elements with the same key value
// are considered — so a renamed server is detected as a change rather than
// silently matched to a different element.
func matchServerElement(configEl interface{}, server []interface{}, claimed []bool) int {
	key, keyVal, hasKey := naturalKey(configEl)
	for i, pe := range server {
		if claimed[i] {
			continue
		}
		if hasKey && !hasNaturalKeyValue(pe, key, keyVal) {
			continue
		}
		if valueIsSubset(configEl, pe) {
			return i
		}
	}
	return -1
}

// naturalKey returns the disambiguating key for an array element: an MCP entry
// is keyed by "mcp_server_name"; a pre-built toolset (type, no server name) is
// keyed by "type". Anything else has no natural key.
func naturalKey(v interface{}) (key, value string, ok bool) {
	m, isObj := v.(map[string]interface{})
	if !isObj {
		return "", "", false
	}
	if name, isStr := m["mcp_server_name"].(string); isStr {
		return "mcp_server_name", name, true
	}
	if _, hasName := m["mcp_server_name"]; !hasName {
		if t, isStr := m["type"].(string); isStr {
			return "type", t, true
		}
	}
	return "", "", false
}

func hasNaturalKeyValue(v interface{}, key, value string) bool {
	m, ok := v.(map[string]interface{})
	if !ok {
		return false
	}
	s, ok := m[key].(string)
	return ok && s == value
}
