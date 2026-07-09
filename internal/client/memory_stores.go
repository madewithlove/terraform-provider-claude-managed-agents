package client

import (
	"context"
	"net/url"
)

// MemoryStore is a workspace-scoped, named container for agent memories.
type MemoryStore struct {
	ID          string            `json:"id"`
	Type        string            `json:"type"`
	Name        string            `json:"name"`
	Description *string           `json:"description"`
	Metadata    map[string]string `json:"metadata"`
	CreatedAt   string            `json:"created_at"`
	UpdatedAt   string            `json:"updated_at"`
	ArchivedAt  *string           `json:"archived_at"`
}

// MemoryStoreCreateRequest is the POST /v1/memory_stores body.
type MemoryStoreCreateRequest struct {
	Name        string            `json:"name"`
	Description *string           `json:"description,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// MemoryStoreUpdateRequest is the POST /v1/memory_stores/{id} body. Description
// clears with an empty string; Metadata is a patch (nil value deletes a key).
type MemoryStoreUpdateRequest struct {
	Name        *string                `json:"name,omitempty"`
	Description *string                `json:"description,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

func (c *Client) CreateMemoryStore(ctx context.Context, req MemoryStoreCreateRequest) (*MemoryStore, error) {
	var out MemoryStore
	if err := c.do(ctx, "POST", "/v1/memory_stores", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetMemoryStore(ctx context.Context, id string) (*MemoryStore, error) {
	var out MemoryStore
	if err := c.do(ctx, "GET", "/v1/memory_stores/"+url.PathEscape(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateMemoryStore(ctx context.Context, id string, req MemoryStoreUpdateRequest) (*MemoryStore, error) {
	var out MemoryStore
	if err := c.do(ctx, "POST", "/v1/memory_stores/"+url.PathEscape(id), req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ArchiveMemoryStore makes a store read-only (one-way; no unarchive).
func (c *Client) ArchiveMemoryStore(ctx context.Context, id string) (*MemoryStore, error) {
	var out MemoryStore
	if err := c.do(ctx, "POST", "/v1/memory_stores/"+url.PathEscape(id)+"/archive", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteMemoryStore permanently removes a store and all of its memories.
func (c *Client) DeleteMemoryStore(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/v1/memory_stores/"+url.PathEscape(id), nil, nil)
}

// Memory is a single text document at a hierarchical path inside a store.
// Content is populated only when fetched with view=full.
type Memory struct {
	ID               string  `json:"id"`
	Type             string  `json:"type"`
	MemoryStoreID    string  `json:"memory_store_id"`
	Path             string  `json:"path"`
	Content          *string `json:"content"`
	ContentSHA256    string  `json:"content_sha256"`
	ContentSizeBytes int64   `json:"content_size_bytes"`
	MemoryVersionID  string  `json:"memory_version_id"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
}

// MemoryCreateRequest is the POST /v1/memory_stores/{id}/memories body.
type MemoryCreateRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// MemoryPrecondition guards an update against a concurrent write.
type MemoryPrecondition struct {
	Type          string `json:"type"`
	ContentSHA256 string `json:"content_sha256"`
}

// MemoryUpdateRequest is the POST /v1/memory_stores/{id}/memories/{mem} body.
type MemoryUpdateRequest struct {
	Path         *string             `json:"path,omitempty"`
	Content      *string             `json:"content,omitempty"`
	Precondition *MemoryPrecondition `json:"precondition,omitempty"`
}

func (c *Client) CreateMemory(ctx context.Context, storeID string, req MemoryCreateRequest) (*Memory, error) {
	var out Memory
	if err := c.do(ctx, "POST", "/v1/memory_stores/"+url.PathEscape(storeID)+"/memories?view=full", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetMemory(ctx context.Context, storeID, memID string) (*Memory, error) {
	var out Memory
	if err := c.do(ctx, "GET", "/v1/memory_stores/"+url.PathEscape(storeID)+"/memories/"+url.PathEscape(memID)+"?view=full", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateMemory(ctx context.Context, storeID, memID string, req MemoryUpdateRequest) (*Memory, error) {
	var out Memory
	if err := c.do(ctx, "POST", "/v1/memory_stores/"+url.PathEscape(storeID)+"/memories/"+url.PathEscape(memID)+"?view=full", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteMemory(ctx context.Context, storeID, memID string) error {
	return c.do(ctx, "DELETE", "/v1/memory_stores/"+url.PathEscape(storeID)+"/memories/"+url.PathEscape(memID), nil, nil)
}
