package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func fastClient(url string) *Client {
	c := New(url, "fp_tok", "org1")
	c.Sleep = func(time.Duration) {}
	return c
}

func TestEveryRequestPinsTheVersionAndTheToken(t *testing.T) {
	var h http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h = r.Header.Clone()
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c := fastClient(srv.URL)
	if _, err := c.DoObject(context.Background(), http.MethodGet, "/x", nil); err != nil {
		t.Fatal(err)
	}
	if h.Get("Forumulate-Version") != "2027-02-01" || h.Get("Authorization") != "Bearer fp_tok" || h.Get("User-Agent") == "" {
		t.Fatalf("headers = %v", h)
	}
	if h.Get("Idempotency-Key") != "" {
		t.Fatal("a GET carries no Idempotency-Key")
	}
}

func TestPostsCarryAnIdempotencyKeyThatSurvivesARetry(t *testing.T) {
	var keys []string
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		if atomic.AddInt32(&n, 1) == 1 {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"data":{"id":"1"}}`))
	}))
	defer srv.Close()
	out, err := fastClient(srv.URL).DoObject(context.Background(), http.MethodPost, "/things", map[string]any{"a": 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 || keys[0] == "" || keys[0] != keys[1] {
		t.Fatalf("a retried create must repeat its Idempotency-Key, got %v", keys)
	}
	if Unwrap(out, "thing")["id"] != "1" {
		t.Fatalf("unwrap = %v", out)
	}
}

func TestRateLimitIsRetriedAfterRetryAfterAndPutsAreNotRetriedOn5xx(t *testing.T) {
	var n int32
	var slept time.Duration
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			atomic.AddInt32(&n, 1)
			w.WriteHeader(502)
			return
		}
		if atomic.AddInt32(&n, 1) == 1 {
			w.Header().Set("Retry-After", "3")
			w.WriteHeader(429)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	c := fastClient(srv.URL)
	c.Sleep = func(d time.Duration) { slept += d }
	if _, err := c.DoObject(context.Background(), http.MethodGet, "/x", nil); err != nil || slept != 3*time.Second {
		t.Fatalf("err %v slept %v", err, slept)
	}
	atomic.StoreInt32(&n, 0)
	if err := c.Do(context.Background(), http.MethodPut, "/x", map[string]any{}, nil); err == nil || atomic.LoadInt32(&n) != 1 {
		t.Fatalf("a PUT that got a 502 must not be blindly repeated (calls %d, err %v)", n, err)
	}
}

func TestErrorEnvelopeKeepsParamHintDetailsAndRequestID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(422)
		_, _ = w.Write([]byte(`{"error":{"code":"validation.failed","message":"Validation failed.","param":"config.pageMounts.0.path",
			"hint":"A mount is / or one segment.","details":{"config.pageMounts.0.path":["bad path"]},"docs":"GET /x","reason":"format"},"requestId":"req_9"}`))
	}))
	defer srv.Close()
	err := fastClient(srv.URL).Do(context.Background(), http.MethodPatch, "/x", map[string]any{}, nil)
	e, ok := AsAPIError(err)
	if !ok || e.Status != 422 || e.Code != "validation.failed" || e.Param != "config.pageMounts.0.path" ||
		e.Hint != "A mount is / or one segment." || e.RequestID != "req_9" || e.Details["config.pageMounts.0.path"][0] != "bad path" || e.Reason != "format" {
		t.Fatalf("err = %#v", err)
	}
	for _, want := range []string{"validation.failed", "config.pageMounts.0.path", "req_9", "A mount is"} {
		if !contains(err.Error(), want) {
			t.Errorf("message %q lacks %q", err.Error(), want)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestNonJSONErrorBodiesAreKept(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte("error code: 1010"))
	}))
	defer srv.Close()
	err := fastClient(srv.URL).Do(context.Background(), http.MethodGet, "/x", nil, nil)
	if e, ok := AsAPIError(err); !ok || e.Status != 403 || e.Message != "error code: 1010" {
		t.Fatalf("err = %#v", err)
	}
	if IsNotFound(err) || IsConflict(err) {
		t.Fatal("classification")
	}
}

func TestListFollowsCursorsAndReadsDataOrTheNamedKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("cursor") {
		case "":
			if r.URL.Query().Get("limit") != "100" || r.URL.Query().Get("locale") != "es" {
				t.Errorf("query = %v", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"data":[{"id":"a"},{"id":"b"}],"page":{"hasMore":true,"nextCursor":"c2"}}`))
		case "c2":
			_, _ = w.Write([]byte(`{"data":[{"id":"c"}],"page":{"hasMore":false,"nextCursor":null}}`))
		}
	}))
	defer srv.Close()
	rows, err := fastClient(srv.URL).List(context.Background(), "/things", "things", map[string][]string{"locale": {"es"}})
	if err != nil || len(rows) != 3 || rows[2]["id"] != "c" {
		t.Fatalf("rows = %v err %v", rows, err)
	}
	legacy := RowsOf(Object{"things": []any{map[string]any{"id": "z"}}}, "things")
	if len(legacy) != 1 {
		t.Fatal("an answer without data must be read by its named key")
	}
	if got := NotFound("page x"); !IsNotFound(got) {
		t.Fatal("NotFound must classify as 404")
	}
}

func TestPaths(t *testing.T) {
	c := New("https://x/api", "", "org 1")
	cases := map[string]string{
		c.OrgPath("webhooks", "a/b"):             "/orgs/org%201/webhooks/a%2Fb",
		c.SpacePath("docs", "forum", "settings"): "/orgs/org%201/spaces/docs/forum/settings",
		c.SitePath("main"):                       "/orgs/org%201/sites/main",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("got %q want %q", got, want)
		}
	}
}
