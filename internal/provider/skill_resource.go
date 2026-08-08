package provider

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/madewithlove/terraform-provider-claude-managed-agents/internal/client"
)

var (
	_ resource.Resource                = (*skillResource)(nil)
	_ resource.ResourceWithConfigure   = (*skillResource)(nil)
	_ resource.ResourceWithImportState = (*skillResource)(nil)
)

type skillResource struct {
	client *client.Client
}

// NewSkillResource is the resource factory.
func NewSkillResource() resource.Resource { return &skillResource{} }

type skillResourceModel struct {
	ID            types.String `tfsdk:"id"`
	SourceDir     types.String `tfsdk:"source_dir"`
	SourceHash    types.String `tfsdk:"source_hash"`
	DisplayTitle  types.String `tfsdk:"display_title"`
	LatestVersion types.String `tfsdk:"latest_version"`
	Type          types.String `tfsdk:"type"`
	CreatedAt     types.String `tfsdk:"created_at"`
}

func (r *skillResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_skill"
}

func (r *skillResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A custom (workspace-authored) skill, uploaded from a local directory " +
			"(a `SKILL.md` plus any supporting files) as a zip bundle. Reference the resulting `id` from a " +
			"`claude_agent`'s `skills` array, e.g. `{type = \"custom\", skill_id = <id>, version = \"latest\"}`. " +
			"`terraform destroy` deletes the skill.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"source_dir": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Path to the skill directory (must contain `SKILL.md`). Its contents are " +
					"zipped with `SKILL.md` at the archive root and uploaded.",
			},
			"source_hash": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "A hash of the directory's contents (compute it in HCL, e.g. a sha256 over " +
					"the files). Changing it re-zips `source_dir` and uploads a new skill version; a stable value " +
					"means no new version.",
			},
			"display_title": schema.StringAttribute{
				Optional:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
				MarkdownDescription: "Optional unique display title. Omit to derive it from `SKILL.md`. Changing " +
					"it replaces the skill.",
			},
			"latest_version": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The most recent version identifier. Use `\"latest\"` (or this value) when attaching to an agent.",
			},
			"type": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"created_at": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *skillResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("Expected *client.Client, got %T.", req.ProviderData))
		return
	}
	r.client = c
}

func (r *skillResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan skillResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	zipData, err := zipSkillDir(plan.SourceDir.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error packaging skill", err.Error())
		return
	}

	skill, err := r.client.CreateSkill(ctx, zipData, "skill.zip", plan.DisplayTitle.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating skill", err.Error())
		return
	}

	applySkill(&plan, skill)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *skillResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state skillResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	skill, err := r.client.GetSkill(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading skill", err.Error())
		return
	}

	applySkill(&state, skill)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *skillResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state skillResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// source_hash (or source_dir) changed → re-zip the current contents and
	// upload a new version. The skill id is stable across versions.
	zipData, err := zipSkillDir(plan.SourceDir.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error packaging skill", err.Error())
		return
	}

	skill, err := r.client.CreateSkillVersion(ctx, state.ID.ValueString(), zipData, "skill.zip")
	if err != nil {
		resp.Diagnostics.AddError("Error creating skill version", err.Error())
		return
	}

	plan.ID = state.ID
	applySkill(&plan, skill)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *skillResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state skillResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteSkill(ctx, state.ID.ValueString())
	if err == nil || client.IsNotFound(err) {
		return
	}
	resp.Diagnostics.AddError("Error deleting skill", err.Error())
}

func (r *skillResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// applySkill copies server-derived fields into the model. source_dir,
// source_hash, and display_title are config-driven and left untouched.
func applySkill(m *skillResourceModel, s *client.Skill) {
	m.ID = types.StringValue(s.ID)
	m.Type = types.StringValue(s.Type)
	m.LatestVersion = types.StringValue(s.LatestVersionString())
	m.CreatedAt = types.StringValue(s.CreatedAt)
}

// zipSkillDir walks dir and returns a zip archive with each file at a path
// relative to dir (so SKILL.md sits at the archive root).
func zipSkillDir(dir string) ([]byte, error) {
	if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil {
		return nil, fmt.Errorf("%s must contain a SKILL.md: %w", dir, err)
	}

	// The Skills API requires a single top-level folder containing SKILL.md
	// (e.g. "writing-style/SKILL.md"), not SKILL.md at the zip root. Nest every
	// entry under a folder named after the source directory.
	root := filepath.Base(dir)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	walkErr := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		w, err := zw.Create(filepath.ToSlash(filepath.Join(root, rel)))
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	})
	if walkErr != nil {
		return nil, walkErr
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
