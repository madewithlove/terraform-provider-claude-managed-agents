package provider

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/madewithlove/terraform-provider-claude-managed-agents/internal/jsontype"
)

// ---- subset-suppress (string / JSON) ----

// subsetSuppressModifier suppresses a spurious plan diff on a JSON attribute
// whenever the config value is a recursive subset of the prior state value.
// This is what makes an imported resource plan cleanly: the API enriches JSON
// on read, so the prior state holds the enriched value while config holds the
// user's subset; without this, the difference would show every plan. When the
// config is NOT a subset (a genuine change), the normal diff is left intact.
//
// Semantic equality on the custom type keeps managed state stable across
// refreshes, but semantic equality does not run during plan — so this plan-time
// modifier is required for the imported (prior != config-shaped) case.
type subsetSuppressModifier struct{}

func (m subsetSuppressModifier) Description(_ context.Context) string {
	return "Suppresses diffs when the config JSON is a subset of the enriched server JSON."
}

func (m subsetSuppressModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m subsetSuppressModifier) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.StateValue.IsNull() || req.StateValue.IsUnknown() {
		return // create, or import before the first read: nothing to compare against.
	}
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() || req.PlanValue.IsUnknown() {
		return
	}
	ok, err := jsontype.IsSubset([]byte(req.ConfigValue.ValueString()), []byte(req.StateValue.ValueString()))
	if err != nil {
		return // invalid JSON: let the normal diff surface it.
	}
	if ok {
		resp.PlanValue = req.StateValue
	}
}

// suppressJSONSubset keeps the (enriched) prior state value when config is a
// subset of it, so imported JSON attributes plan cleanly.
func suppressJSONSubset() planmodifier.String { return subsetSuppressModifier{} }

// ---- requires-replace-if-known-and-changed ----

// requiresReplaceIfKnownChanged forces replacement only when the prior state
// value is known and non-null and differs from the plan. On import the prior
// state is null (or, for fields the API cannot return, stays null), so no
// replacement is triggered and the config value is adopted on the next apply.
// Genuine post-adoption changes to these immutable fields still force replace.
func requiresReplaceIfKnownChanged() planmodifier.String {
	return stringplanmodifier.RequiresReplaceIf(
		func(_ context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
			if req.StateValue.IsNull() || req.StateValue.IsUnknown() {
				resp.RequiresReplace = false
				return
			}
			resp.RequiresReplace = !req.StateValue.Equal(req.PlanValue)
		},
		"Requires replacement only when the prior value is known and changes.",
		"Requires replacement only when the prior value is known and changes.",
	)
}

// ---- environment config (object): subset-suppress or replace ----

// envConfigModifier makes the immutable environment `config` object plan
// cleanly on import while still forcing replacement on a genuine change:
//   - prior state null/unknown (create or pre-first-read import): no-op;
//   - config is a recursive subset of the enriched prior state: suppress the
//     diff by keeping the prior value;
//   - otherwise: force replacement.
type envConfigModifier struct{}

func (m envConfigModifier) Description(_ context.Context) string {
	return "Suppresses enrichment diffs on config; forces replacement on a genuine change."
}

func (m envConfigModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m envConfigModifier) PlanModifyObject(ctx context.Context, req planmodifier.ObjectRequest, resp *planmodifier.ObjectResponse) {
	if req.StateValue.IsNull() || req.StateValue.IsUnknown() {
		return
	}
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	var stateBlock, cfgBlock envConfigBlock
	resp.Diagnostics.Append(req.StateValue.As(ctx, &stateBlock, basetypes.ObjectAsOptions{})...)
	resp.Diagnostics.Append(req.ConfigValue.As(ctx, &cfgBlock, basetypes.ObjectAsOptions{})...)
	if resp.Diagnostics.HasError() {
		return
	}

	stateAPI, d := envConfigToAPI(ctx, &stateBlock)
	resp.Diagnostics.Append(d...)
	cfgAPI, d := envConfigToAPI(ctx, &cfgBlock)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	stateJSON, err := json.Marshal(stateAPI)
	if err != nil {
		return
	}
	cfgJSON, err := json.Marshal(cfgAPI)
	if err != nil {
		return
	}

	ok, err := jsontype.IsSubset(cfgJSON, stateJSON)
	if err != nil {
		return
	}
	if ok {
		resp.PlanValue = req.StateValue
		return
	}
	resp.RequiresReplace = true
}

// envConfigSubsetOrReplace suppresses enrichment diffs and forces replacement
// on genuine changes to the immutable environment config.
func envConfigSubsetOrReplace() planmodifier.Object { return envConfigModifier{} }
