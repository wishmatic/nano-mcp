package mcp

import (
	"slices"
	"testing"
)

func TestNewBuildsAServer(t *testing.T) {
	srv, err := New(noopDeps())
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	if srv == nil {
		t.Fatal("New() = nil, want a server")
	}
}

func TestServerInfoNamesThisRepo(t *testing.T) {
	session := connectedSession(t, noopDeps())

	info := session.InitializeResult().ServerInfo
	if info.Name != "nano-mcp" || info.Version != version {
		t.Errorf("server info = %+v, want nano-mcp %s", info, version)
	}
}

func TestServerWithoutAClientListsNoTools(t *testing.T) {
	session := connectedSession(t, noopDeps())

	if names := toolNames(t, session); len(names) != 0 {
		t.Errorf("tools = %v, want none until a nano-gpt client is configured", names)
	}
}

func TestServerListsTheTools(t *testing.T) {
	deps := noopDeps()
	deps.NanoGPT = nanoClient(t, "http://127.0.0.1:1")

	session := connectedSession(t, deps)

	want := []string{"firecrawl_crawl", "firecrawl_map", "firecrawl_scrape", "web_scrape", "web_search"}
	names := toolNames(t, session)
	slices.Sort(names)

	if !slices.Equal(names, want) {
		t.Errorf("tools = %v, want %v", names, want)
	}
}
