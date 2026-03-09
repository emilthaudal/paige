// Package opencode provides an HTTP client for interacting with the OpenCode
// server API (opencode serve).
//
// API reference: https://opencode.ai/docs/server
package opencode

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const defaultBaseURL = "http://localhost:4096"

// Client talks to a running OpenCode server instance.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// Option is a functional option for Client.
type Option func(*Client)

// WithBaseURL overrides the default server URL.
func WithBaseURL(url string) Option {
	return func(c *Client) { c.baseURL = url }
}

// WithTimeout overrides the default HTTP timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.httpClient.Timeout = d }
}

// NewClient creates a new OpenCode API client.
func NewClient(opts ...Option) *Client {
	c := &Client{
		baseURL:    defaultBaseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// --- Types matching the OpenCode API ---

// Session represents an OpenCode session.
type Session struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// MessagePart is a single content part of a message (text, tool call, etc.).
type MessagePart struct {
	Type    string `json:"type"`
	Text    string `json:"text,omitempty"`
	Partial bool   `json:"partial,omitempty"`
}

// MessageInfo holds metadata about a message.
type MessageInfo struct {
	ID               string  `json:"id"`
	Role             string  `json:"role"`
	Error            *string `json:"error,omitempty"`
	StructuredOutput any     `json:"structured_output,omitempty"`
}

// MessageResponse is the response from sending a prompt.
type MessageResponse struct {
	Info  MessageInfo   `json:"info"`
	Parts []MessagePart `json:"parts"`
}

// PromptRequest is the body for POST /session/:id/message.
type PromptRequest struct {
	Parts   []MessagePart `json:"parts"`
	NoReply bool          `json:"noReply,omitempty"`
}

// --- API methods ---

// Health checks the server health.
func (c *Client) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/global/health", nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("opencode health check: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("opencode health check: status %d", resp.StatusCode)
	}
	return nil
}

// CreateSession creates a new OpenCode session and returns it.
func (c *Client) CreateSession(ctx context.Context, title string) (Session, error) {
	body, _ := json.Marshal(map[string]string{"title": title})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/session", bytes.NewReader(body))
	if err != nil {
		return Session{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	var s Session
	if err := c.do(req, &s); err != nil {
		return Session{}, fmt.Errorf("create session: %w", err)
	}
	return s, nil
}

// SendPrompt sends a prompt to an existing session and waits for the response.
func (c *Client) SendPrompt(ctx context.Context, sessionID string, prompt string) (MessageResponse, error) {
	pr := PromptRequest{
		Parts: []MessagePart{
			{Type: "text", Text: prompt},
		},
	}
	body, _ := json.Marshal(pr)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/session/"+sessionID+"/message", bytes.NewReader(body))
	if err != nil {
		return MessageResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	var mr MessageResponse
	if err := c.do(req, &mr); err != nil {
		return MessageResponse{}, fmt.Errorf("send prompt: %w", err)
	}
	return mr, nil
}

// DeleteSession deletes an OpenCode session.
func (c *Client) DeleteSession(ctx context.Context, sessionID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		c.baseURL+"/session/"+sessionID, nil)
	if err != nil {
		return err
	}
	return c.do(req, nil)
}

// ExtractText concatenates all text parts from a MessageResponse into a single string.
func ExtractText(mr MessageResponse) string {
	var buf bytes.Buffer
	for _, p := range mr.Parts {
		if p.Type == "text" {
			buf.WriteString(p.Text)
		}
	}
	return buf.String()
}

// --- internal ---

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(b))
	}

	if out != nil {
		return json.Unmarshal(b, out)
	}
	return nil
}
