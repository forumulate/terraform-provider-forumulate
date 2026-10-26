// Package client is the provider's small HTTP client for the Forumulate API:
// a base URL, a personal access token, JSON in and out, and the API's error
// envelope turned into Go errors that say what the server said.
package client

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultAPIURL is the hosted API, the same default the CLI and SDKs use.
const DefaultAPIURL = "https://forumulate.com/api"

// APIVersion is the `Forumulate-Version` the provider pins. It is the newest
// version and includes every one before it: a taken page slug is a 409 instead
// of a silent suffix, a body with an unknown key is a 422 instead of a silent
// drop, and lists answer their rows once under `data`. A provider must never
// have a field it sent quietly ignored, which is why it pins this.
const APIVersion = "2027-02-01"

// maxAttempts bounds the retries on a 429 or a gateway error.
const maxAttempts = 4

// Client talks to one organization's API with one token.
type Client struct {
	BaseURL   string
	Token     string
	OrgID     string
	UserAgent string
	HTTP      *http.Client
	// Sleep waits between retries; tests replace it so they do not wait.
	Sleep func(time.Duration)
}

// New returns a client with a 30-second timeout: the API's edge cuts a request
// at 30 s anyway, and a hung apply is worse than a clear error.
func New(baseURL, token, orgID string) *Client {
	return &Client{
		BaseURL:   strings.TrimRight(baseURL, "/"),
		Token:     token,
		OrgID:     orgID,
		UserAgent: "terraform-provider-forumulate",
		HTTP:      &http.Client{Timeout: 30 * time.Second},
		Sleep:     time.Sleep,
	}
}

// WithTimeout is a copy of the client whose requests may take as long as
// `d`. The site manifest's apply is the one call that legitimately outlasts 30 s.
func (c *Client) WithTimeout(d time.Duration) *Client {
	cp := *c
	cp.HTTP = &http.Client{Timeout: d}
	return &cp
}

// APIError is a non-2xx answer: its status and the API's own envelope
// (`{error: {code, message, param, hint, details, ...}}`).
type APIError struct {
	Status    int
	Code      string
	Message   string
	Param     string
	Hint      string
	Reason    string
	Expected  string
	Received  string
	Docs      string
	Details   map[string][]string
	RequestID string
}

func (e *APIError) Error() string {
	var b strings.Builder
	if e.Code != "" {
		fmt.Fprintf(&b, "forumulate API %d %s: %s", e.Status, e.Code, e.Message)
	} else {
		fmt.Fprintf(&b, "forumulate API %d: %s", e.Status, e.Message)
	}
	if e.Param != "" {
		fmt.Fprintf(&b, " (param: %s)", e.Param)
	}
	if e.Hint != "" {
		fmt.Fprintf(&b, " Hint: %s", e.Hint)
	}
	if e.RequestID != "" {
		fmt.Fprintf(&b, " [request %s]", e.RequestID)
	}
	return b.String()
}

// AsAPIError unwraps err to an *APIError, when it is one.
func AsAPIError(err error) (*APIError, bool) {
	var e *APIError
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

// IsNotFound reports a 404, which Read turns into "remove from state".
func IsNotFound(err error) bool {
	e, ok := AsAPIError(err)
	return ok && e.Status == http.StatusNotFound
}

// IsConflict reports a 409.
func IsConflict(err error) bool {
	e, ok := AsAPIError(err)
	return ok && e.Status == http.StatusConflict
}

// NotFound is the 404 the provider raises itself when a list has no such row
// (there is no single-GET for many resources, so Read filters a list).
func NotFound(what string) error {
	return &APIError{Status: http.StatusNotFound, Code: "not_found", Message: what + " not found"}
}

// OrgPath is `/orgs/{org}` plus the segments, each escaped.
func (c *Client) OrgPath(rest ...string) string {
	return join("orgs", append([]string{c.OrgID}, rest...))
}

// SpacePath is `/orgs/{org}/spaces/{space}` plus the segments.
func (c *Client) SpacePath(spaceID string, rest ...string) string {
	return join("orgs", append([]string{c.OrgID, "spaces", spaceID}, rest...))
}

// SitePath is `/orgs/{org}/sites/{site}` plus the segments.
func (c *Client) SitePath(siteID string, rest ...string) string {
	return join("orgs", append([]string{c.OrgID, "sites", siteID}, rest...))
}

func join(first string, rest []string) string {
	parts := []string{first}
	for _, r := range rest {
		parts = append(parts, url.PathEscape(r))
	}
	return "/" + strings.Join(parts, "/")
}

// StatusPath is `/orgs/{org}/spaces/{space}/status` plus the rest, escaped.
func (c *Client) StatusPath(spaceID string, rest ...string) string {
	return c.SpacePath(spaceID, append([]string{"status"}, rest...)...)
}

// Do sends `body` (JSON, when not nil) and decodes a 2xx answer into `out`
// (when not nil).
func (c *Client) Do(ctx context.Context, method, path string, body, out any) error {
	raw, _, err := c.send(ctx, method, path, body, "application/json")
	if err != nil {
		return err
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	return dec.Decode(out)
}

// Object is a decoded JSON object. Numbers stay json.Number so an integer
// the API sent is not turned into a float and back.
type Object = map[string]any

// DoObject is Do decoded into an Object ({} on an empty answer).
func (c *Client) DoObject(ctx context.Context, method, path string, body any) (Object, error) {
	var out Object
	if err := c.Do(ctx, method, path, body, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = Object{}
	}
	return out, nil
}

// DoText fetches a plain-text answer (the Markdown endpoints).
func (c *Client) DoText(ctx context.Context, path string) (string, error) {
	raw, _, err := c.send(ctx, http.MethodGet, path, nil, "text/markdown, text/plain, application/json")
	return string(raw), err
}

// List walks a collection and answers every row. Under the pinned version
// rows are under `data`; the named `key` is read as well so a server that
// predates the lean list shape still works. It follows `page.nextCursor`.
func (c *Client) List(ctx context.Context, path, key string, query url.Values) ([]Object, error) {
	var rows []Object
	cursor := ""
	for i := 0; i < 1000; i++ {
		q := url.Values{}
		for k, v := range query {
			q[k] = v
		}
		q.Set("limit", "100")
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		var out Object
		if err := c.Do(ctx, http.MethodGet, path+"?"+q.Encode(), nil, &out); err != nil {
			return nil, err
		}
		page := RowsOf(out, key)
		rows = append(rows, page...)
		next := ""
		if p, ok := out["page"].(Object); ok {
			if more, _ := p["hasMore"].(bool); more {
				if s, ok := p["nextCursor"].(string); ok {
					next = s
				}
			}
		}
		if next == "" || len(page) == 0 {
			return rows, nil
		}
		cursor = next
	}
	return rows, nil
}

// RowsOf is the rows of a list answer: `data`, else the named key.
func RowsOf(out Object, key string) []Object {
	for _, k := range []string{"data", key} {
		if arr, ok := out[k].([]any); ok {
			rows := make([]Object, 0, len(arr))
			for _, r := range arr {
				if o, ok := r.(Object); ok {
					rows = append(rows, o)
				}
			}
			return rows
		}
	}
	return nil
}

// Unwrap picks the resource out of a create/update answer: the named key,
// else `data`, else the answer itself.
func Unwrap(out Object, key string) Object {
	if o, ok := out[key].(Object); ok {
		return o
	}
	if o, ok := out["data"].(Object); ok {
		return o
	}
	return out
}

func newIdempotencyKey() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "tf-" + hex.EncodeToString(b)
}

func (c *Client) send(ctx context.Context, method, path string, body any, accept string) ([]byte, http.Header, error) {
	var payload []byte
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, nil, err
		}
		payload = buf
	}
	// A create carries an Idempotency-Key, so a retry after a timeout cannot
	// make a second space or webhook.
	idem := ""
	if method == http.MethodPost {
		idem = newIdempotencyKey()
	}
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		var rd io.Reader
		if payload != nil {
			rd = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rd)
		if err != nil {
			return nil, nil, err
		}
		req.Header.Set("Accept", accept)
		req.Header.Set("Forumulate-Version", APIVersion)
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if idem != "" {
			req.Header.Set("Idempotency-Key", idem)
		}
		if c.Token != "" {
			req.Header.Set("Authorization", "Bearer "+c.Token)
		}
		req.Header.Set("User-Agent", c.UserAgent)
		res, err := c.HTTP.Do(req)
		if err != nil {
			// A transport failure is retried only when repeating it is safe.
			lastErr = err
			if (method == http.MethodGet || idem != "") && attempt < maxAttempts && ctx.Err() == nil {
				c.Sleep(backoff(attempt, ""))
				continue
			}
			return nil, nil, err
		}
		raw, rerr := io.ReadAll(io.LimitReader(res.Body, 32<<20))
		res.Body.Close()
		if rerr != nil {
			return nil, nil, rerr
		}
		if res.StatusCode >= 200 && res.StatusCode <= 299 {
			return raw, res.Header, nil
		}
		apiErr := decodeError(res.StatusCode, raw)
		if apiErr.RequestID == "" {
			apiErr.RequestID = res.Header.Get("X-Request-Id")
		}
		retryable := res.StatusCode == http.StatusTooManyRequests ||
			((res.StatusCode == 502 || res.StatusCode == 503 || res.StatusCode == 504) && (method == http.MethodGet || idem != ""))
		if retryable && attempt < maxAttempts {
			lastErr = apiErr
			c.Sleep(backoff(attempt, res.Header.Get("Retry-After")))
			continue
		}
		return nil, nil, apiErr
	}
	return nil, nil, lastErr
}

func backoff(attempt int, retryAfter string) time.Duration {
	if n, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && n >= 0 {
		if n > 30 {
			n = 30
		}
		return time.Duration(n) * time.Second
	}
	return time.Duration(attempt*attempt) * 500 * time.Millisecond
}

// decodeError reads the API's `{error: {...}, requestId}` envelope. A body
// that is not that envelope (an edge page, an HTML 502) is kept verbatim.
func decodeError(status int, raw []byte) *APIError {
	e := &APIError{Status: status, Message: strings.TrimSpace(string(raw))}
	var env struct {
		Error struct {
			Code      string              `json:"code"`
			Message   string              `json:"message"`
			Param     string              `json:"param"`
			Hint      string              `json:"hint"`
			Reason    string              `json:"reason"`
			Expected  string              `json:"expected"`
			Received  string              `json:"received"`
			Docs      string              `json:"docs"`
			Details   map[string][]string `json:"details"`
			RequestID string              `json:"requestId"`
		} `json:"error"`
		// The older flat spelling the provider's first release read.
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"requestId"`
	}
	if json.Unmarshal(raw, &env) != nil {
		return e
	}
	x := env.Error
	if x.Code == "" {
		x.Code = env.Code
	}
	if x.Message == "" {
		x.Message = env.Message
	}
	if x.Code == "" && x.Message == "" {
		return e
	}
	e.Code, e.Message = x.Code, x.Message
	e.Param, e.Hint, e.Reason, e.Expected, e.Received, e.Docs = x.Param, x.Hint, x.Reason, x.Expected, x.Received, x.Docs
	e.Details = x.Details
	e.RequestID = x.RequestID
	if e.RequestID == "" {
		e.RequestID = env.RequestID
	}
	return e
}
