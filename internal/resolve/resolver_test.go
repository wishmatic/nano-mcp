package resolve

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func newResolver(t *testing.T) *Client {
	t.Helper()

	return New(nil)
}

func TestFetchFollowsRedirectToPlainURL(t *testing.T) {
	r := New(nil)

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte("from-http"))
	}))
	defer origin.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, origin.URL+"/x.png", http.StatusMovedPermanently)
	}))
	defer redirector.Close()

	data, err := r.Fetch(context.Background(), redirector.URL)
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	if string(data) != "from-http" {
		t.Errorf("data = %q, want from-http", data)
	}
}

func TestFetchSetsUserAgent(t *testing.T) {
	var (
		mu     sync.Mutex
		agents []string
	)

	record := func(req *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		agents = append(agents, req.UserAgent())
	}

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		record(req)
		_, _ = w.Write([]byte("from-http"))
	}))
	defer origin.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		record(req)
		http.Redirect(w, req, origin.URL+"/x.png", http.StatusFound)
	}))
	defer redirector.Close()

	if _, err := New(nil).Fetch(context.Background(), redirector.URL+"/start"); err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(agents) != 2 {
		t.Fatalf("requests = %d, want one per redirect hop", len(agents))
	}

	for i, agent := range agents {
		if !strings.Contains(agent, "nano-mcp") || !strings.Contains(agent, "bot") {
			t.Errorf("hop %d User-Agent = %q, want a descriptive agent naming nano-mcp and bot", i, agent)
		}
	}
}

func TestFetchRejectsARedirectWithoutALocation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()

	_, err := New(nil).Fetch(context.Background(), server.URL+"/nowhere.png")
	if err == nil || !strings.Contains(err.Error(), "Location") {
		t.Errorf("Fetch() error = %v, want it to name the missing Location", err)
	}
}

func TestFetchGivesUpOnARedirectLoop(t *testing.T) {
	var server *httptest.Server

	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, server.URL, http.StatusFound)
	}))
	defer server.Close()

	_, err := New(nil).Fetch(context.Background(), server.URL+"/loop.png")
	if err == nil || !strings.Contains(err.Error(), "too many redirects") {
		t.Errorf("Fetch() error = %v, want the hop limit named", err)
	}
}

func TestFetchNonOK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	if _, err := New(nil).Fetch(context.Background(), server.URL+"/missing.png"); err == nil {
		t.Fatal("Fetch() expected error, got nil")
	}
}

func TestFetchRejectsAMalformedURL(t *testing.T) {
	if _, err := New(nil).Fetch(context.Background(), "://nope"); err == nil {
		t.Fatal("Fetch() expected error, got nil")
	}
}
