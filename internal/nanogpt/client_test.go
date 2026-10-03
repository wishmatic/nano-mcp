package nanogpt

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

const testKey = "nano-key-123"

func TestNewRejectsAMissingKey(t *testing.T) {
	if _, err := New(Config{BaseURL: "http://127.0.0.1:1"}); !errors.Is(err, ErrNoAPIKey) {
		t.Errorf("New() error = %v, want ErrNoAPIKey", err)
	}
}

func TestNewAppliesTheDefaultBaseURL(t *testing.T) {
	c, err := New(Config{APIKey: testKey})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	if c.baseURL != DefaultBaseURL {
		t.Errorf("baseURL = %q, want %q", c.baseURL, DefaultBaseURL)
	}
}

func TestNewTrimsATrailingSlash(t *testing.T) {
	c, err := New(Config{APIKey: testKey, BaseURL: "http://198.51.100.7:8080/"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	if c.baseURL != "http://198.51.100.7:8080" {
		t.Errorf("baseURL = %q, want the trailing slash dropped", c.baseURL)
	}
}

func TestSearchReportsUpstreamFailures(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusTooManyRequests, http.StatusInternalServerError} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			srv, _ := recordingServer(t, `{"error":"upstream said no"}`, status)

			_, err := newTestClient(t, srv.URL).Search(context.Background(), SearchRequest{Query: "q"})
			if err == nil {
				t.Fatal("Search() error = nil, want the upstream status surfaced")
			}

			if !strings.Contains(err.Error(), strconv.Itoa(status)) {
				t.Errorf("error = %q, want it to name the status", err)
			}

			if !strings.Contains(err.Error(), "upstream said no") {
				t.Errorf("error = %q, want the response body quoted", err)
			}
		})
	}
}

func TestScrapeReportsUpstreamFailures(t *testing.T) {
	srv, _ := recordingServer(t, `{"error":"out of credit"}`, http.StatusPaymentRequired)

	_, err := newTestClient(t, srv.URL).Scrape(context.Background(), ScrapeRequest{URLs: []string{"https://example.com"}})
	if err == nil || !strings.Contains(err.Error(), "402") || !strings.Contains(err.Error(), "out of credit") {
		t.Errorf("Scrape() error = %v, want the status and body surfaced", err)
	}
}

func TestSearchRejectsAMalformedResponse(t *testing.T) {
	srv, _ := recordingServer(t, `{"data":`, http.StatusOK)

	_, err := newTestClient(t, srv.URL).Search(context.Background(), SearchRequest{Query: "q"})
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Errorf("Search() error = %v, want a decode error", err)
	}
}

func newTestClient(t *testing.T, baseURL string) *Client {
	t.Helper()

	c, err := New(Config{APIKey: testKey, BaseURL: baseURL})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	return c
}

type recordedRequest struct {
	method  string
	path    string
	headers http.Header
	body    map[string]any
}

func recordingServer(t *testing.T, response string, status int) (*httptest.Server, *recordedRequest) {
	t.Helper()

	rec := &recordedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.method = r.Method
		rec.path = r.URL.Path
		rec.headers = r.Header.Clone()

		if err := json.NewDecoder(r.Body).Decode(&rec.body); err != nil {
			t.Errorf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)

		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(srv.Close)

	return srv, rec
}

func (r *recordedRequest) assertHeader(t *testing.T, name, want string) {
	t.Helper()

	if got := r.headers.Get(name); got != want {
		t.Errorf("%s = %q, want %q", name, got, want)
	}
}

func (r *recordedRequest) assertBody(t *testing.T, key string, want any) {
	t.Helper()

	got, ok := r.body[key]
	if !ok {
		t.Fatalf("body has no %q: %v", key, r.body)
	}

	if got != want {
		t.Errorf("body[%q] = %v, want %v", key, got, want)
	}
}

func (r *recordedRequest) assertBodyNumber(t *testing.T, key string, want float64) {
	t.Helper()

	got, ok := r.body[key]
	if !ok {
		t.Fatalf("body has no %q: %v", key, r.body)
	}

	if number, ok := got.(float64); !ok || number != want {
		t.Errorf("body[%q] = %v, want %v", key, got, want)
	}
}

func (r *recordedRequest) assertBodyOmits(t *testing.T, key string) {
	t.Helper()

	if got, ok := r.body[key]; ok && got != nil {
		t.Errorf("body[%q] = %v, want it omitted", key, got)
	}
}
