package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func mustMap(t *testing.T, m map[string]string) types.Map {
	t.Helper()
	if m == nil {
		return types.MapNull(types.StringType)
	}
	v, diags := types.MapValueFrom(context.Background(), types.StringType, m)
	if diags.HasError() {
		t.Fatalf("building map: %v", diags)
	}
	return v
}

func TestMergedMetadata(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name  string
		state map[string]string
		plan  map[string]string
		want  map[string]interface{}
	}{
		{
			name:  "add and update keys",
			state: map[string]string{"a": "1"},
			plan:  map[string]string{"a": "2", "b": "3"},
			want:  map[string]interface{}{"a": "2", "b": "3"},
		},
		{
			name:  "removed key is nulled",
			state: map[string]string{"a": "1", "b": "2"},
			plan:  map[string]string{"a": "1"},
			want:  map[string]interface{}{"a": "1", "b": nil},
		},
		{
			name:  "clear all nulls every key",
			state: map[string]string{"a": "1", "b": "2"},
			plan:  nil,
			want:  map[string]interface{}{"a": nil, "b": nil},
		},
		{
			name:  "both empty sends nothing",
			state: nil,
			plan:  nil,
			want:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, diags := mergedMetadata(ctx, mustMap(t, tc.state), mustMap(t, tc.plan))
			if diags.HasError() {
				t.Fatalf("unexpected diags: %v", diags)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("length mismatch: got %v want %v", got, tc.want)
			}
			for k, wv := range tc.want {
				gv, ok := got[k]
				if !ok {
					t.Fatalf("missing key %q in %v", k, got)
				}
				if gv != wv {
					t.Fatalf("key %q: got %v want %v", k, gv, wv)
				}
			}
		})
	}
}

func TestScheduleTypeDefaultsToCron(t *testing.T) {
	if got := scheduleType(nil); got != "cron" {
		t.Fatalf("nil schedule: got %q want cron", got)
	}
	b := &deploymentScheduleBlock{Type: types.StringNull()}
	if got := scheduleType(b); got != "cron" {
		t.Fatalf("null type: got %q want cron", got)
	}
	b = &deploymentScheduleBlock{Type: types.StringValue("cron")}
	if got := scheduleType(b); got != "cron" {
		t.Fatalf("explicit: got %q want cron", got)
	}
}

func TestStatusToPaused(t *testing.T) {
	if !statusToPaused("paused") {
		t.Fatal("paused status should map to true")
	}
	if statusToPaused("active") {
		t.Fatal("active status should map to false")
	}
}

func TestNormalizedFromRaw(t *testing.T) {
	if !normalizedFromRaw(nil).IsNull() {
		t.Fatal("nil raw should be null")
	}
	if !normalizedFromRaw([]byte("null")).IsNull() {
		t.Fatal("JSON null should be null")
	}
	n := normalizedFromRaw([]byte(`{"a":1}`))
	if n.IsNull() {
		t.Fatal("object should not be null")
	}
}
