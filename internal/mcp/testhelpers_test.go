package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wishmatic/nano-mcp/internal/filestore"
	"github.com/wishmatic/nano-mcp/internal/nanogpt"
	"go.uber.org/zap"
)

const testNanoKey = "nano-key-123"

func noopDeps() Deps {
	return Deps{Log: zap.NewNop()}
}

func nanoClient(t *testing.T, baseURL string) *nanogpt.Client {
	t.Helper()

	c, err := nanogpt.New(nanogpt.Config{APIKey: testNanoKey, BaseURL: baseURL})
	if err != nil {
		t.Fatalf("nanogpt.New() error: %v", err)
	}

	return c
}

const testPublicHost = "https://nano.example.com"

// videoStore keeps assets in a directory that goes away with the test, and serves them from a
// host the test can compare against rather than one that would need to resolve. It returns the
// directory too, so a test can check what actually landed on disk.
func videoStore(t *testing.T) (*filestore.Client, string) {
	t.Helper()

	base, err := url.Parse(testPublicHost)
	if err != nil {
		t.Fatalf("url.Parse() error: %v", err)
	}

	dir := filepath.Join(t.TempDir(), "files")

	client, err := filestore.New(filestore.Config{Dir: dir, PublicBase: base}, zap.NewNop())
	if err != nil {
		t.Fatalf("filestore.New() error: %v", err)
	}

	return client, dir
}

func fileStore(t *testing.T) *filestore.Client {
	t.Helper()

	client, _ := videoStore(t)

	return client
}

func upstream(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return srv.URL
}

func jsonUpstream(t *testing.T, path, body string) string {
	t.Helper()

	return upstream(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			t.Errorf("upstream path = %s, want %s", r.URL.Path, path)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})
}

func connectedSession(t *testing.T, deps Deps) *mcp.ClientSession {
	t.Helper()

	srv, err := New(deps)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	serverSession, err := srv.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server Connect() error: %v", err)
	}

	t.Cleanup(func() { _ = serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0"}, nil)

	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client Connect() error: %v", err)
	}

	t.Cleanup(func() { _ = session.Close() })

	return session
}

func nanoSession(t *testing.T, baseURL string) *mcp.ClientSession {
	t.Helper()

	deps := noopDeps()
	deps.NanoGPT = nanoClient(t, baseURL)
	deps.Files = fileStore(t)

	return connectedSession(t, deps)
}

func toolNames(t *testing.T, session *mcp.ClientSession) []string {
	t.Helper()

	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools() error: %v", err)
	}

	names := make([]string, 0, len(result.Tools))
	for _, tool := range result.Tools {
		names = append(names, tool.Name)
	}

	return names
}

func findTool(t *testing.T, session *mcp.ClientSession, name string) *mcp.Tool {
	t.Helper()

	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools() error: %v", err)
	}

	for _, tool := range result.Tools {
		if tool.Name == name {
			return tool
		}
	}

	t.Fatalf("tool %q is not listed", name)

	return nil
}

type schemaProperty struct {
	Description string   `json:"description"`
	Enum        []string `json:"enum"`
}

// toolProperties decodes a tool's input schema the way a client sees it, which arrives as
// raw JSON rather than the SDK's typed schema.
func toolProperties(t *testing.T, tool *mcp.Tool) map[string]schemaProperty {
	t.Helper()

	raw, err := json.Marshal(tool.InputSchema)
	if err != nil {
		t.Fatalf("marshal input schema: %v", err)
	}

	var schema struct {
		Properties map[string]schemaProperty `json:"properties"`
	}

	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("decode input schema %s: %v", raw, err)
	}

	return schema.Properties
}

func callTool(t *testing.T, session *mcp.ClientSession, name string, args any) *mcp.CallToolResult {
	t.Helper()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool(%s) error: %v", name, err)
	}

	return result
}

func structured(t *testing.T, result *mcp.CallToolResult, out any) {
	t.Helper()

	if result.StructuredContent == nil {
		t.Fatal("result carries no structured content")
	}

	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}

	if err := json.Unmarshal(raw, out); err != nil {
		t.Fatalf("decode structured content %s: %v", raw, err)
	}
}

func errorText(result *mcp.CallToolResult) string {
	var text strings.Builder

	for _, content := range result.Content {
		if block, ok := content.(*mcp.TextContent); ok {
			text.WriteString(block.Text)
		}
	}

	return text.String()
}
