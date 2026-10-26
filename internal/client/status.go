package client

import (
	"context"
	"encoding/json"
	"net/http"
)

// Monitor is a status page component as the API sends it (MonitorSummary).
// Only the fields the provider manages are decoded.
type Monitor struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	Kind             string          `json:"kind"`
	URL              string          `json:"url"`
	Method           string          `json:"method"`
	IntervalSeconds  int64           `json:"intervalSeconds"`
	Visible          bool            `json:"visible"`
	AlertsEnabled    bool            `json:"alertsEnabled"`
	FailureThreshold int64           `json:"failureThreshold"`
	SuccessThreshold int64           `json:"successThreshold"`
	Group            *string         `json:"group"`
	Keyword          *string         `json:"keyword"`
	KeywordAbsent    bool            `json:"keywordAbsent"`
	Paused           bool            `json:"paused"`
	Probe            json.RawMessage `json:"probe"`
	Regions          []string        `json:"regions"`
	ConfirmRegions   int64           `json:"confirmRegions"`
	BarDays          *int64          `json:"barDays"`
	ShowUptime       bool            `json:"showUptime"`
	CertExpiryDays   *int64          `json:"certExpiryDays"`
	ExpectStatus     *int64          `json:"expectStatus"`
	Description      *string         `json:"description"`
	Slug             *string         `json:"slug"`
	LinkURL          *string         `json:"linkUrl"`
	Display          string          `json:"display"`
	Region           *string         `json:"region"`
	PingURL          string          `json:"pingUrl,omitempty"`
}

// ListMonitors is every component of a status page.
func (c *Client) ListMonitors(ctx context.Context, spaceID string) ([]Monitor, error) {
	return listAs[Monitor](ctx, c, c.StatusPath(spaceID, "monitors"), "monitors")
}

// GetMonitor finds one component; there is no single-monitor GET, so it is the
// list filtered, and a missing id is an APIError 404 like any other.
func (c *Client) GetMonitor(ctx context.Context, spaceID, id string) (*Monitor, error) {
	all, err := c.ListMonitors(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].ID == id {
			return &all[i], nil
		}
	}
	return nil, &APIError{Status: http.StatusNotFound, Code: "not_found", Message: "status component " + id + " not found"}
}

// CreateMonitor posts a component body (the API's monitorSchema).
func (c *Client) CreateMonitor(ctx context.Context, spaceID string, body map[string]any) (*Monitor, error) {
	var out struct {
		Monitor Monitor `json:"monitor"`
	}
	if err := c.Do(ctx, http.MethodPost, c.StatusPath(spaceID, "monitors"), body, &out); err != nil {
		return nil, err
	}
	return &out.Monitor, nil
}

// UpdateMonitor patches a component.
func (c *Client) UpdateMonitor(ctx context.Context, spaceID, id string, body map[string]any) (*Monitor, error) {
	var out struct {
		Monitor Monitor `json:"monitor"`
	}
	if err := c.Do(ctx, http.MethodPatch, c.StatusPath(spaceID, "monitors", id), body, &out); err != nil {
		return nil, err
	}
	return &out.Monitor, nil
}

// DeleteMonitor removes a component and its history.
func (c *Client) DeleteMonitor(ctx context.Context, spaceID, id string) error {
	return c.Do(ctx, http.MethodDelete, c.StatusPath(spaceID, "monitors", id), nil, nil)
}

// Group is one entry of a page's group arrangement.
type Group struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	Parent      *string `json:"parent,omitempty"`
	Collapsed   bool    `json:"collapsed,omitempty"`
	Layout      *string `json:"layout,omitempty"`
	HideUptime  bool    `json:"hideUptime,omitempty"`
}

// GetGroups is the page's group arrangement, in order.
func (c *Client) GetGroups(ctx context.Context, spaceID string) ([]Group, error) {
	var out struct {
		Groups []Group `json:"groups"`
	}
	if err := c.Do(ctx, http.MethodGet, c.StatusPath(spaceID, "groups"), nil, &out); err != nil {
		return nil, err
	}
	return out.Groups, nil
}

// SetGroups replaces the arrangement.
func (c *Client) SetGroups(ctx context.Context, spaceID string, groups []Group) ([]Group, error) {
	var out struct {
		Groups []Group `json:"groups"`
	}
	body := map[string]any{"groups": groups}
	if err := c.Do(ctx, http.MethodPut, c.StatusPath(spaceID, "groups"), body, &out); err != nil {
		return nil, err
	}
	return out.Groups, nil
}

// Template is an incident template.
type Template struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Title    string            `json:"title"`
	Severity string            `json:"severity"`
	State    string            `json:"state"`
	Updates  map[string]string `json:"updates"`
}

// ListTemplates is every incident template of the page.
func (c *Client) ListTemplates(ctx context.Context, spaceID string) ([]Template, error) {
	return listAs[Template](ctx, c, c.StatusPath(spaceID, "templates"), "templates")
}

// GetTemplate finds one template in the list.
func (c *Client) GetTemplate(ctx context.Context, spaceID, id string) (*Template, error) {
	all, err := c.ListTemplates(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].ID == id {
			return &all[i], nil
		}
	}
	return nil, &APIError{Status: http.StatusNotFound, Code: "not_found", Message: "incident template " + id + " not found"}
}

// CreateTemplate posts a template; `markdown` carries its write-up.
func (c *Client) CreateTemplate(ctx context.Context, spaceID string, body map[string]any) (*Template, error) {
	var out struct {
		Template Template `json:"template"`
	}
	if err := c.Do(ctx, http.MethodPost, c.StatusPath(spaceID, "templates"), body, &out); err != nil {
		return nil, err
	}
	return &out.Template, nil
}

// UpdateTemplate patches a template.
func (c *Client) UpdateTemplate(ctx context.Context, spaceID, id string, body map[string]any) (*Template, error) {
	var out struct {
		Template Template `json:"template"`
	}
	if err := c.Do(ctx, http.MethodPatch, c.StatusPath(spaceID, "templates", id), body, &out); err != nil {
		return nil, err
	}
	return &out.Template, nil
}

// DeleteTemplate removes a template.
func (c *Client) DeleteTemplate(ctx context.Context, spaceID, id string) error {
	return c.Do(ctx, http.MethodDelete, c.StatusPath(spaceID, "templates", id), nil, nil)
}

// Metric is a custom metric's settings.
type Metric struct {
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Unit        string   `json:"unit"`
	Description *string  `json:"description"`
	Decimals    int64    `json:"decimals"`
	Aggregate   string   `json:"aggregate"`
	Visible     bool     `json:"visible"`
	SortKey     int64    `json:"sortKey"`
	AxisMin     *float64 `json:"axisMin"`
	AxisMax     *float64 `json:"axisMax"`
	Threshold   *float64 `json:"threshold"`
}

// ListMetrics is every custom metric of the page.
func (c *Client) ListMetrics(ctx context.Context, spaceID string) ([]Metric, error) {
	return listAs[Metric](ctx, c, c.StatusPath(spaceID, "metrics"), "metrics")
}

// GetMetric finds one metric by key.
func (c *Client) GetMetric(ctx context.Context, spaceID, key string) (*Metric, error) {
	all, err := c.ListMetrics(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].Key == key {
			return &all[i], nil
		}
	}
	return nil, &APIError{Status: http.StatusNotFound, Code: "not_found", Message: "metric " + key + " not found"}
}

// PutMetric creates or patches a metric.
func (c *Client) PutMetric(ctx context.Context, spaceID, key string, body map[string]any) (*Metric, error) {
	var out struct {
		Metric Metric `json:"metric"`
	}
	if err := c.Do(ctx, http.MethodPut, c.StatusPath(spaceID, "metrics", key), body, &out); err != nil {
		return nil, err
	}
	return &out.Metric, nil
}

// DeleteMetric removes a metric and every point of it.
func (c *Client) DeleteMetric(ctx context.Context, spaceID, key string) error {
	return c.Do(ctx, http.MethodDelete, c.StatusPath(spaceID, "metrics", key), nil, nil)
}

// PageState is the public `widget.json` answer, trimmed to what the data
// source exposes.
type PageState struct {
	Page struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"page"`
	Status struct {
		Indicator   string `json:"indicator"`
		Description string `json:"description"`
		Overall     string `json:"overall"`
	} `json:"status"`
	Active []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Kind   string `json:"kind"`
		Status string `json:"status"`
		Impact string `json:"impact"`
		URL    string `json:"url"`
	} `json:"active"`
}

// GetPageState reads the page's public state (no token needed).
func (c *Client) GetPageState(ctx context.Context, spaceID string) (*PageState, error) {
	var out PageState
	path := "/public" + c.StatusPath(spaceID, "widget.json")
	if err := c.Do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// listAs reads a list answer whose rows are `data` (the pinned version) or the
// named key (older servers), decoded into T.
func listAs[T any](ctx context.Context, c *Client, path, key string) ([]T, error) {
	var out map[string]json.RawMessage
	if err := c.Do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	raw, ok := out["data"]
	if !ok {
		raw = out[key]
	}
	var rows []T
	if len(raw) == 0 {
		return rows, nil
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}
