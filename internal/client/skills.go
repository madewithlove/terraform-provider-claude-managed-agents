package client

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/url"
	"strings"
)

// Skill is a custom (workspace-authored) skill registered via the Skills API.
type Skill struct {
	ID            string          `json:"id"`
	Type          string          `json:"type"`
	DisplayTitle  *string         `json:"display_title"`
	LatestVersion json.RawMessage `json:"latest_version"`
	CreatedAt     string          `json:"created_at"`
}

// LatestVersionString renders latest_version as a string whether the API sends
// it as a JSON string ("1") or a number (1).
func (s *Skill) LatestVersionString() string {
	if len(s.LatestVersion) == 0 || string(s.LatestVersion) == "null" {
		return ""
	}
	return strings.Trim(string(s.LatestVersion), `"`)
}

// CreateSkill uploads a zipped skill bundle (SKILL.md plus any supporting files
// at the archive root) and returns the new skill's id + latest version.
func (c *Client) CreateSkill(ctx context.Context, zipData []byte, zipName, displayTitle string) (*Skill, error) {
	return c.uploadSkill(ctx, "POST", "/v1/skills", zipData, zipName, displayTitle)
}

// CreateSkillVersion uploads a new version of an existing skill.
func (c *Client) CreateSkillVersion(ctx context.Context, id string, zipData []byte, zipName string) (*Skill, error) {
	return c.uploadSkill(ctx, "POST", "/v1/skills/"+url.PathEscape(id)+"/versions", zipData, zipName, "")
}

// GetSkill fetches a custom skill by id.
func (c *Client) GetSkill(ctx context.Context, id string) (*Skill, error) {
	var out Skill
	if err := c.doRaw(ctx, "GET", "/v1/skills/"+url.PathEscape(id), SkillsBetaHeader, "", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteSkill permanently removes a custom skill.
func (c *Client) DeleteSkill(ctx context.Context, id string) error {
	return c.doRaw(ctx, "DELETE", "/v1/skills/"+url.PathEscape(id), SkillsBetaHeader, "", nil, nil)
}

// uploadSkill posts a multipart/form-data body with the zip under files[] and,
// optionally, a display_title field, against the Skills beta.
func (c *Client) uploadSkill(ctx context.Context, method, path string, zipData []byte, zipName, displayTitle string) (*Skill, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("files[]", zipName)
	if err != nil {
		return nil, err
	}
	if _, err := fw.Write(zipData); err != nil {
		return nil, err
	}
	if displayTitle != "" {
		if err := mw.WriteField("display_title", displayTitle); err != nil {
			return nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}

	var out Skill
	if err := c.doRaw(ctx, method, path, SkillsBetaHeader, mw.FormDataContentType(), &buf, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
