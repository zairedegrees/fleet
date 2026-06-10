package relay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	url    string
	client *http.Client
}

type Agent struct {
	Name        string `json:"name"`
	Role        string `json:"role"`
	Status      string `json:"status"`
	ProfileSlug string `json:"profile_slug"`
	ReportsTo   string `json:"reports_to"`
	IsExecutive bool   `json:"is_executive"`
	Color       string `json:"color"`
}

type Profile struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
	Role string `json:"role"`
}

type mpcRequest struct {
	Jsonrpc string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type mpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mpcResponse struct {
	Jsonrpc string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *mpcError       `json:"error"`
}

func NewClient(url string) *Client {
	return NewClientWithTimeout(url, 10*time.Second)
}

// NewClientWithTimeout builds a client with a custom HTTP timeout. Used by
// callers that need a snappier probe than the default (e.g. the doctor).
func NewClientWithTimeout(url string, timeout time.Duration) *Client {
	return &Client{
		url: url,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) call(toolName string, args map[string]interface{}) (json.RawMessage, error) {
	params := map[string]interface{}{
		"name":      toolName,
		"arguments": args,
	}
	paramsJSON, _ := json.Marshal(params)

	req := mpcRequest{
		Jsonrpc: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  paramsJSON,
	}

	body, _ := json.Marshal(req)
	resp, err := c.client.Post(c.url, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("relay unreachable: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("relay returned HTTP %d: %s", resp.StatusCode, bytes.TrimSpace(data))
	}

	var mpcResp mpcResponse
	if err := json.Unmarshal(data, &mpcResp); err != nil {
		return nil, fmt.Errorf("invalid relay response: %w", err)
	}
	if mpcResp.Error != nil {
		return nil, fmt.Errorf("relay error %d: %s", mpcResp.Error.Code, mpcResp.Error.Message)
	}

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(mpcResp.Result, &result); err != nil {
		return nil, err
	}
	if len(result.Content) == 0 {
		return nil, fmt.Errorf("empty relay response")
	}
	if result.IsError {
		return nil, fmt.Errorf("relay tool error: %s", result.Content[0].Text)
	}
	return json.RawMessage(result.Content[0].Text), nil
}

func (c *Client) ListProjects() ([]string, error) {
	// The relay has no list_projects endpoint.
	// We discover projects by listing all agents (no project filter)
	// and extracting unique project names.
	data, err := c.call("list_agents", map[string]interface{}{})
	if err != nil {
		return nil, err
	}
	var result struct {
		Agents []struct {
			Project string `json:"project"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	var projects []string
	for _, a := range result.Agents {
		if a.Project != "" && !seen[a.Project] {
			seen[a.Project] = true
			projects = append(projects, a.Project)
		}
	}
	return projects, nil
}

func (c *Client) ListAgents(project string) ([]Agent, error) {
	data, err := c.call("list_agents", map[string]interface{}{"project": project})
	if err != nil {
		return nil, err
	}
	var result struct {
		Agents []Agent `json:"agents"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result.Agents, nil
}

func (c *Client) ListProfiles(project string) ([]Profile, error) {
	data, err := c.call("list_profiles", map[string]interface{}{"project": project})
	if err != nil {
		return nil, err
	}
	var result struct {
		Profiles []Profile `json:"profiles"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result.Profiles, nil
}

func (c *Client) DispatchTask(agent, project, description string) error {
	_, err := c.call("dispatch_task", map[string]interface{}{
		"assignee":    agent,
		"project":     project,
		"title":       description,
		"description": description,
		"priority":    "high",
	})
	return err
}

func (c *Client) Health() error {
	_, err := c.call("list_orgs", map[string]interface{}{})
	return err
}

// DeactivateAgent deregisters an agent from the relay so it no longer appears in
// list_agents or task routing. Called by `fleet stop` to avoid ghost agents.
func (c *Client) DeactivateAgent(name, project string) error {
	_, err := c.call("deactivate_agent", map[string]interface{}{
		"name":    name,
		"project": project,
	})
	return err
}

// RegisterAgent registers an agent on the relay with a profile_slug — the slug
// is what lets dispatched tasks route to the agent. Mirrors the inline curl the
// launch configure script issues.
func (c *Client) RegisterAgent(name, project, role, profileSlug string) error {
	_, err := c.call("register_agent", map[string]interface{}{
		"name":         name,
		"project":      project,
		"role":         role,
		"profile_slug": profileSlug,
	})
	return err
}

// EnsureProfile creates or updates a profile on the relay.
func (c *Client) EnsureProfile(name, role, project string) error {
	_, err := c.call("register_profile", map[string]interface{}{
		"slug":    name,
		"name":    name,
		"role":    role,
		"project": project,
	})
	return err
}

// PushVaultDoc pushes a vault document to the relay for a specific project.
// Uses set_memory with scope "project" to store the doc content.
func (c *Client) PushVaultDoc(project, path string, content []byte) error {
	_, err := c.call("set_memory", map[string]interface{}{
		"key":     "vault:" + path,
		"value":   string(content),
		"scope":   "project",
		"project": project,
		"tags":    []string{"vault", "auto-injected"},
	})
	return err
}
