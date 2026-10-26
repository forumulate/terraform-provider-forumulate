package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStatusPathEscapesEverySegment(t *testing.T) {
	c := New("https://x.example/api/", "tok", "org 1")
	if got := c.StatusPath("sp/1", "metrics", "p99 ms"); got != "/orgs/org%201/spaces/sp%2F1/status/metrics/p99%20ms" {
		t.Fatalf("path = %q", got)
	}
	if c.BaseURL != "https://x.example/api" {
		t.Fatalf("base = %q", c.BaseURL)
	}
}

func TestDoSendsTokenAndDecodesErrors(t *testing.T) {
	var seen *http.Request
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		switch r.URL.Path {
		case "/orgs/o/spaces/s/status/monitors":
			if r.Method == http.MethodPost {
				w.WriteHeader(201)
				_, _ = w.Write([]byte(`{"monitor":{"id":"m1","name":"API","kind":"http","regions":["eu"],"barDays":30,"probe":{"host":"db"}}}`))
				return
			}
			_, _ = w.Write([]byte(`{"monitors":[{"id":"m1","name":"API"}]}`))
		default:
			w.WriteHeader(422)
			_, _ = w.Write([]byte(`{"code":"validation.failed","message":"Name is required."}`))
		}
	}))
	defer srv.Close()
	c := New(srv.URL, "fp_secret", "o")
	m, err := c.CreateMonitor(context.Background(), "s", map[string]any{"name": "API"})
	if err != nil {
		t.Fatal(err)
	}
	if seen.Header.Get("Authorization") != "Bearer fp_secret" || seen.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("headers = %v", seen.Header)
	}
	if body["name"] != "API" || m.ID != "m1" || *m.BarDays != 30 || m.Regions[0] != "eu" || string(m.Probe) != `{"host":"db"}` {
		t.Fatalf("monitor = %+v, body = %v", m, body)
	}
	if _, err := c.GetMonitor(context.Background(), "s", "missing"); !IsNotFound(err) {
		t.Fatalf("missing monitor err = %v", err)
	}
	err = c.Do(context.Background(), http.MethodGet, "/nope", nil, nil)
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Status != 422 || apiErr.Code != "validation.failed" || apiErr.Message != "Name is required." {
		t.Fatalf("err = %#v", err)
	}
}

func TestPageStateIsThePublicWidgetAnswer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/public/orgs/o/spaces/s/status/widget.json" || r.Header.Get("Authorization") != "" {
			w.WriteHeader(404)
			return
		}
		_, _ = w.Write([]byte(`{"page":{"name":"Acme","url":"https://status.acme"},"status":{"indicator":"major","description":"Trouble","overall":"down"},"active":[{"id":"i1"}]}`))
	}))
	defer srv.Close()
	st, err := New(srv.URL, "", "o").GetPageState(context.Background(), "s")
	if err != nil {
		t.Fatal(err)
	}
	if st.Status.Overall != "down" || len(st.Active) != 1 || st.Page.Name != "Acme" {
		t.Fatalf("state = %+v", st)
	}
}
