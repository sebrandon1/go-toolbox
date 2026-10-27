package httpx

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name   string
		header string
		want   time.Duration
	}{
		{"empty", "", 0},
		{"seconds", "5", 5 * time.Second},
		{"zero seconds", "0", 0},
		{"negative seconds", "-2", 0},
		{"future date", now.Add(90 * time.Second).Format(http.TimeFormat), 90 * time.Second},
		{"past date", now.Add(-time.Minute).Format(http.TimeFormat), 0},
		{"garbage", "soon", 0},
		{"spaces", "  7  ", 7 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseRetryAfterAt(tc.header, now); got != tc.want {
				t.Fatalf("ParseRetryAfterAt(%q) = %v, want %v", tc.header, got, tc.want)
			}
		})
	}
}

func TestCheckStatus(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/widgets/42", nil)
	cases := []struct {
		name   string
		status int
		header http.Header
		body   string
		check  func(*testing.T, error)
	}{
		{"ok", http.StatusOK, nil, "", func(t *testing.T, err error) {
			if err != nil {
				t.Fatal(err)
			}
		}},
		{"created", http.StatusCreated, nil, "", func(t *testing.T, err error) {
			if err != nil {
				t.Fatal(err)
			}
		}},
		{"no content", http.StatusNoContent, nil, "", func(t *testing.T, err error) {
			if err != nil {
				t.Fatal(err)
			}
		}},
		{"unexpected 2xx", http.StatusIMUsed, nil, "x", func(t *testing.T, err error) {
			if !IsHTTPError(err) {
				t.Fatalf("want HTTPError, got %v", err)
			}
		}},
		{"unauthorized", http.StatusUnauthorized, nil, "nope", func(t *testing.T, err error) {
			if !IsAuthError(err) {
				t.Fatalf("want AuthError, got %v", err)
			}
		}},
		{"not found", http.StatusNotFound, nil, "", func(t *testing.T, err error) {
			var nf *NotFoundError
			if !errors.As(err, &nf) {
				t.Fatalf("want NotFoundError, got %v", err)
			}
			if nf.Resource != "widget" || nf.ID != "42" {
				t.Fatalf("resource=%q id=%q", nf.Resource, nf.ID)
			}
		}},
		{"rate limited", http.StatusTooManyRequests, http.Header{"Retry-After": {"3"}}, "", func(t *testing.T, err error) {
			var rl *RateLimitError
			if !errors.As(err, &rl) {
				t.Fatalf("want RateLimitError, got %v", err)
			}
			if rl.RetryAfter != 3*time.Second {
				t.Fatalf("RetryAfter=%v", rl.RetryAfter)
			}
		}},
		{"validation", http.StatusUnprocessableEntity, nil, `{"name":["required"]}`, func(t *testing.T, err error) {
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("want ValidationError, got %v", err)
			}
			if len(ve.Fields["name"]) != 1 {
				t.Fatalf("fields=%v", ve.Fields)
			}
		}},
		{"validation bad body", http.StatusUnprocessableEntity, nil, "not json", func(t *testing.T, err error) {
			var ve *ValidationError
			if !errors.As(err, &ve) || ve.Fields != nil {
				t.Fatalf("want ValidationError with nil fields, got %v", err)
			}
		}},
		{"server error", http.StatusBadGateway, nil, "upstream", func(t *testing.T, err error) {
			var he *HTTPError
			if !errors.As(err, &he) || he.StatusCode != http.StatusBadGateway || he.Body != "upstream" {
				t.Fatalf("want HTTPError 502, got %v", err)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := &http.Response{StatusCode: tc.status, Header: tc.header, Request: req}
			tc.check(t, CheckStatus(resp, []byte(tc.body)))
		})
	}
}

func TestParseNotFoundPath(t *testing.T) {
	cases := []struct {
		path, resource, id string
	}{
		{"/api/widgets/42", "widget", "42"},
		{"/api/categories", "category", ""},
		{"/api/policies/abc", "policy", "abc"},
		{"", "", ""},
		{"/", "", ""},
	}
	for _, tc := range cases {
		resource, id := ParseNotFoundPath(tc.path)
		if resource != tc.resource || id != tc.id {
			t.Fatalf("ParseNotFoundPath(%q) = %q,%q want %q,%q", tc.path, resource, id, tc.resource, tc.id)
		}
	}
}

func TestAddQueryParams(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x?keep=1&keep=2", nil)
	AddQueryParams(req, map[string]string{"a": "b c", "d": "e"})
	q := req.URL.Query()
	if got := q.Get("keep"); got != "1" {
		t.Fatalf("keep=%q", got)
	}
	if len(q["keep"]) != 2 {
		t.Fatalf("keep values=%v", q["keep"])
	}
	if q.Get("a") != "b c" || q.Get("d") != "e" {
		t.Fatalf("query=%v", q)
	}
	// Empty map is a no-op.
	before := req.URL.RawQuery
	AddQueryParams(req, nil)
	if req.URL.RawQuery != before {
		t.Fatalf("raw query changed: %q", req.URL.RawQuery)
	}
}

func TestNewRequestWithBody(t *testing.T) {
	req, err := NewRequestWithBody(context.Background(), http.MethodPost, "http://example.com", map[string]int{"n": 1})
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"n":1}` {
		t.Fatalf("body=%q", body)
	}

	req, err = NewRequestWithBody(context.Background(), http.MethodGet, "http://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	if req.Body != nil {
		t.Fatal("expected nil body")
	}

	if _, err := NewRequestWithBody(context.Background(), http.MethodPost, "http://example.com", make(chan int)); err == nil {
		t.Fatal("expected marshal error")
	}
}

func TestPathAndJoin(t *testing.T) {
	if got := PathSeg("a/b c"); got != "a%2Fb%20c" {
		t.Fatalf("PathSeg=%q", got)
	}
	if got := JoinPath("https://example.com/api/", "widgets", "42"); got != "https://example.com/api/widgets/42" {
		t.Fatalf("JoinPath=%q", got)
	}
	if got := JoinPath("https://example.com/api"); got != "https://example.com/api" {
		t.Fatalf("JoinPath base=%q", got)
	}
}

func TestRedactURLQuery(t *testing.T) {
	u, err := url.Parse("https://user:pass@example.com/x?api_token=abc&keep=1&password=z")
	if err != nil {
		t.Fatal(err)
	}
	got := RedactURLQuery(u)
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"api_token", "password"} {
		if v := parsed.Query().Get(key); v != "[REDACTED]" {
			t.Fatalf("%s not redacted: %q (url %s)", key, v, got)
		}
	}
	if parsed.User != nil {
		t.Fatalf("user info not removed: %s", got)
	}
	if parsed.Query().Get("keep") != "1" {
		t.Fatalf("lost non-sensitive query: %s", got)
	}
	if RedactURLQuery(nil) != "<unknown>" {
		t.Fatal("nil URL should be <unknown>")
	}
}

func TestReadLimited(t *testing.T) {
	if _, err := ReadLimited(strings.NewReader("abcdef"), 3); err == nil {
		t.Fatal("expected size error")
	}
	b, err := ReadLimited(strings.NewReader("abc"), 3)
	if err != nil || string(b) != "abc" {
		t.Fatalf("b=%q err=%v", b, err)
	}
	// Non-positive limit falls back to the default cap.
	b, err = ReadLimited(strings.NewReader("abc"), 0)
	if err != nil || string(b) != "abc" {
		t.Fatalf("b=%q err=%v", b, err)
	}
}

func TestReadErrorBodyTruncates(t *testing.T) {
	if got := ReadErrorBody(strings.NewReader(strings.Repeat("x", 20)), 5); got != "xxxxx" {
		t.Fatalf("got %q", got)
	}
}

func TestClientDoJSON(t *testing.T) {
	var gotAuth, gotUA, gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotUA = r.Header.Get("User-Agent")
		gotContentType = r.Header.Get("Content-Type")
		switch r.URL.Path {
		case "/api/widgets":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"items":[1,2]}`))
		case "/api/empty":
			w.WriteHeader(http.StatusNoContent)
		case "/api/missing":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	client := NewClient(Options{
		BaseURL:   srv.URL + "/api",
		UserAgent: "go-toolbox-test",
		Headers:   http.Header{"Authorization": {"Bearer t"}},
	})

	var out struct {
		Items []int `json:"items"`
	}
	if err := client.DoJSON(context.Background(), http.MethodGet, "widgets", nil, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Items) != 2 {
		t.Fatalf("items=%v", out.Items)
	}
	if gotAuth != "Bearer t" || gotUA != "go-toolbox-test" {
		t.Fatalf("auth=%q ua=%q", gotAuth, gotUA)
	}
	if gotContentType != "" {
		t.Fatalf("unexpected content type on bodyless GET: %q", gotContentType)
	}

	if err := client.DoJSON(context.Background(), http.MethodPost, "widgets", map[string]int{"a": 1}, nil); err != nil {
		t.Fatal(err)
	}
	if gotContentType != "application/json" {
		t.Fatalf("content type=%q", gotContentType)
	}

	if err := client.DoJSON(context.Background(), http.MethodGet, "empty", nil, nil); err != nil {
		t.Fatal(err)
	}

	err := client.DoJSON(context.Background(), http.MethodGet, "missing", nil, nil)
	if !IsNotFound(err) {
		t.Fatalf("want NotFound, got %v", err)
	}

	client.BaseURL = srv.URL + "/api"
	err = client.DoJSON(context.Background(), http.MethodGet, "boom", nil, nil)
	var he *HTTPError
	if !errors.As(err, &he) || he.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500 HTTPError, got %v", err)
	}
}

func TestClientDoJSONNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()

	client := NewClient(Options{BaseURL: url})
	err := client.DoJSON(context.Background(), http.MethodGet, "/", nil, nil)
	if !IsNetworkError(err) {
		t.Fatalf("want NetworkError, got %v", err)
	}
}

func TestClientSetHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Per-Request") != "yes" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client := NewClient(Options{BaseURL: srv.URL})
	client.SetHeaders = func(r *http.Request) { r.Header.Set("X-Per-Request", "yes") }
	if err := client.DoJSON(context.Background(), http.MethodGet, "/", nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestClientURL(t *testing.T) {
	client := NewClient(Options{BaseURL: "https://example.com/api"})
	if got := client.URL("widgets"); got != "https://example.com/api/widgets" {
		t.Fatalf("URL=%q", got)
	}
	if got := client.URL("https://other.example/x"); got != "https://other.example/x" {
		t.Fatalf("absolute URL=%q", got)
	}
	empty := NewClient(Options{})
	if got := empty.URL("rel"); got != "rel" {
		t.Fatalf("empty base URL=%q", got)
	}
}

func TestNewHTTPClientDefaults(t *testing.T) {
	c := NewHTTPClient(Options{})
	if c.Timeout != DefaultTimeout {
		t.Fatalf("timeout=%v", c.Timeout)
	}
	if c.Transport == nil {
		t.Fatal("nil transport")
	}
	custom := &http.Transport{}
	c = NewHTTPClient(Options{Transport: custom, Timeout: time.Second})
	if c.Transport != custom || c.Timeout != time.Second {
		t.Fatal("custom transport/timeout not applied")
	}
}

func TestHandleResponseNil(t *testing.T) {
	if err := HandleResponse(nil, nil, 0); !IsNetworkError(err) {
		t.Fatalf("want NetworkError, got %v", err)
	}
}

func TestAsNetworkError(t *testing.T) {
	if got := AsNetworkError(nil); got != nil {
		t.Fatalf("nil: %v", got)
	}
	if got := AsNetworkError(context.Canceled); !errors.Is(got, context.Canceled) {
		t.Fatalf("cancel: %v", got)
	}
	existing := &NetworkError{Cause: errors.New("x")}
	if got := AsNetworkError(existing); !errors.Is(got, existing) {
		t.Fatal("should not double-wrap")
	}
	if got := AsNetworkError(errors.New("x")); !IsNetworkError(got) {
		t.Fatalf("want wrap, got %v", got)
	}
}
