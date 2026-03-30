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

type mpcResponse struct {
	Jsonrpc string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result"`
}

func NewClient(url string) *Client {
	return &Client{
		url: url,
		client: &http.Client{
			Timeout: 10 * time.Second,
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
	var mpcResp mpcResponse
	if err := json.Unmarshal(data, &mpcResp); err != nil {
		return nil, fmt.Errorf("invalid relay response: %w", err)
	}

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(mpcResp.Result, &result); err != nil {
		return nil, err
	}
	if len(result.Content) == 0 {
		return nil, fmt.Errorf("empty relay response")
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

func (c *Client) Health() error {
	_, err := c.call("list_orgs", map[string]interface{}{})
	return err
}
