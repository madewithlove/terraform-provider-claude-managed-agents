// Package jsontype provides a JSON string custom type whose semantic equality
// tolerates server-side enrichment.
//
// The Claude Managed Agents API enriches some JSON fields on the way back: a
// config value like `[{"type":"agent_toolset_20260401"}]` returns as
// `[{"type":"agent_toolset_20260401","default_config":{...}}]`. A byte-for-byte
// (or even property-order-normalized) comparison would then flag a perpetual
// diff. Subset treats two JSON documents as equal when the config-side value is
// a recursive subset of the server-side value: extra object keys added by the
// server are ignored, but any key/element the config specifies must be present
// and match.
package jsontype

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/attr/xattr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// ---- Type ----

var _ basetypes.StringTypable = (*SubsetType)(nil)

// SubsetType is the attribute type for Subset values.
type SubsetType struct {
	jsontypes.NormalizedType
}

func (t SubsetType) String() string { return "jsontype.SubsetType" }

func (t SubsetType) ValueType(_ context.Context) attr.Value { return Subset{} }

func (t SubsetType) Equal(o attr.Type) bool {
	other, ok := o.(SubsetType)
	if !ok {
		return false
	}
	return t.NormalizedType.Equal(other.NormalizedType)
}

func (t SubsetType) ValueFromString(_ context.Context, in basetypes.StringValue) (basetypes.StringValuable, diag.Diagnostics) {
	return Subset{StringValue: in}, nil
}

func (t SubsetType) ValueFromTerraform(ctx context.Context, in tftypes.Value) (attr.Value, error) {
	attrValue, err := t.NormalizedType.StringType.ValueFromTerraform(ctx, in)
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
	_ basetypes.StringValuable                   = (*Subset)(nil)
	_ basetypes.StringValuableWithSemanticEquals = (*Subset)(nil)
	_ xattr.ValidateableAttribute                = (*Subset)(nil)
)

// Subset is a JSON string value with enrichment-tolerant semantic equality.
type Subset struct {
	basetypes.StringValue
}

func (v Subset) Type(_ context.Context) attr.Type { return SubsetType{} }

func (v Subset) Equal(o attr.Value) bool {
	other, ok := o.(Subset)
	if !ok {
		return false
	}
	return v.StringValue.Equal(other.StringValue)
}

// StringSemanticEquals reports whether the prior value can be kept in place of
// this (proposed new) value.
//
// The framework calls this as proposedNew.StringSemanticEquals(ctx, prior)
// (see terraform-plugin-framework fwschemadata.ValueSemanticEqualityString):
// the receiver v is the freshly produced value (during a refresh, the
// enriched server response), and the argument is the prior state value (the
// user's config-shaped value). Returning true keeps the prior value, which is
// exactly what we want when the prior config is a recursive subset of the
// enriched server value — the two are semantically the same and the config
// value should win to avoid a diff.
func (v Subset) StringSemanticEquals(_ context.Context, priorValuable basetypes.StringValuable) (bool, diag.Diagnostics) {
	var diags diag.Diagnostics

	prior, ok := priorValuable.(Subset)
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

	// Null/unknown on either side: fall back to strict equality; there is no
	// meaningful subset relationship to exploit.
	if v.IsNull() || v.IsUnknown() || prior.IsNull() || prior.IsUnknown() {
		return v.StringValue.Equal(prior.StringValue), diags
	}

	// Keep the prior value when it is a subset of the new (server) value.
	equal, err := IsSubset([]byte(prior.ValueString()), []byte(v.ValueString()))
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
func (v Subset) ValidateAttribute(_ context.Context, req xattr.ValidateAttributeRequest, resp *xattr.ValidateAttributeResponse) {
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

// NewSubsetValue returns a known Subset value.
func NewSubsetValue(value string) Subset {
	return Subset{StringValue: basetypes.NewStringValue(value)}
}

// NewSubsetNull returns a null Subset value.
func NewSubsetNull() Subset {
	return Subset{StringValue: basetypes.NewStringNull()}
}

// ---- Subset comparison ----

// IsSubset reports whether the JSON document sub is a recursive subset of the
// JSON document super:
//   - objects: every key in sub must exist in super and its value must be a
//     recursive subset; extra keys in super are ignored;
//   - arrays: equal length, element-wise recursive subset;
//   - scalars: strictly equal.
func IsSubset(sub, super []byte) (bool, error) {
	subv, err := decodeJSON(sub)
	if err != nil {
		return false, err
	}
	superv, err := decodeJSON(super)
	if err != nil {
		return false, err
	}
	return valueIsSubset(subv, superv), nil
}

func decodeJSON(b []byte) (interface{}, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	// Preserve numeric representation (avoid float64 normalization), matching
	// jsontypes.Normalized behavior.
	dec.UseNumber()
	var v interface{}
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

func valueIsSubset(sub, super interface{}) bool {
	switch s := sub.(type) {
	case map[string]interface{}:
		o, ok := super.(map[string]interface{})
		if !ok {
			return false
		}
		for k, sv := range s {
			ov, ok := o[k]
			if !ok || !valueIsSubset(sv, ov) {
				return false
			}
		}
		return true
	case []interface{}:
		o, ok := super.([]interface{})
		if !ok || len(o) != len(s) {
			return false
		}
		for i := range s {
			if !valueIsSubset(s[i], o[i]) {
				return false
			}
		}
		return true
	case json.Number:
		o, ok := super.(json.Number)
		return ok && s.String() == o.String()
	case string:
		o, ok := super.(string)
		return ok && s == o
	case bool:
		o, ok := super.(bool)
		return ok && s == o
	case nil:
		return super == nil
	default:
		// Should not happen with encoding/json + UseNumber, but be strict.
		return strings.EqualFold(fmt.Sprint(sub), fmt.Sprint(super))
	}
}
