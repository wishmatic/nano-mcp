package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wishmatic/nano-mcp/internal/auth"
	"github.com/wishmatic/nano-mcp/internal/config"
	"github.com/wishmatic/nano-mcp/internal/filestore"
	"github.com/wishmatic/nano-mcp/internal/nanogpt"
	"go.uber.org/zap"
)

const testPublicHost = "https://nano.example.com"

func testConfig(t *testing.T) config.Config {
	t.Helper()

	return config.Config{
		Port:          8080,
		APIKey:        "server-key",
		NanoGPTAPIKey: "nano-key",
		PublicHost:    testPublicHost,
		FilesDir:      filepath.Join(t.TempDir(), "files"),
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()

	srv, err := New(testConfig(t), zap.NewNop())
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	return srv
}

func TestShutdown(t *testing.T) {
	if err := newTestServer(t).Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error: %v", err)
	}
}

func TestNewRequiresAPIKey(t *testing.T) {
	cfg := testConfig(t)
	cfg.APIKey = ""

	_, err := New(cfg, zap.NewNop())
	if !errors.Is(err, auth.ErrNoAPIKey) {
		t.Errorf("New() error = %v, want auth.ErrNoAPIKey", err)
	}
}

func TestNewRequiresNanoGPTAPIKey(t *testing.T) {
	cfg := testConfig(t)
	cfg.NanoGPTAPIKey = ""

	_, err := New(cfg, zap.NewNop())
	if !errors.Is(err, nanogpt.ErrNoAPIKey) {
		t.Errorf("New() error = %v, want nanogpt.ErrNoAPIKey", err)
	}
}

func TestNewRejectsAnInvalidPort(t *testing.T) {
	cfg := testConfig(t)
	cfg.Port = 0

	_, err := New(cfg, zap.NewNop())
	if err == nil || !strings.Contains(err.Error(), "PORT") {
		t.Errorf("New() error = %v, want it to name PORT", err)
	}
}

func TestNewRequiresPublicHost(t *testing.T) {
	for name, publicHost := range map[string]string{
		"unset":       "",
		"no host":     "https://",
		"no scheme":   "nano.example.com",
		"with a path": "https://nano.example.com/files",
	} {
		t.Run(name, func(t *testing.T) {
			cfg := testConfig(t)
			cfg.PublicHost = publicHost

			_, err := New(cfg, zap.NewNop())
			if err == nil || !strings.Contains(err.Error(), "PUBLIC_HOST") {
				t.Errorf("New() error = %v, want it to name PUBLIC_HOST", err)
			}
		})
	}
}

func TestNewRejectsABadImageURLMap(t *testing.T) {
	cfg := testConfig(t)
	cfg.ImageURLMap = "https://chat.example.com/images"

	_, err := New(cfg, zap.NewNop())
	if err == nil || !strings.Contains(err.Error(), "IMAGE_URL_MAP") {
		t.Errorf("New() error = %v, want it to name IMAGE_URL_MAP", err)
	}
}

func TestWriteTimeoutOutlivesTheUpstreamCall(t *testing.T) {
	if writeTimeout <= nanogpt.MaxCallBudget {
		t.Errorf("writeTimeout = %s, want it above nanogpt.MaxCallBudget (%s)", writeTimeout, nanogpt.MaxCallBudget)
	}

	if longest := nanogpt.VideoWaitBudget + filestore.MaxFetchDuration; writeTimeout <= longest {
		t.Errorf("writeTimeout = %s, want it above a video call's %s", writeTimeout, longest)
	}

	if image := nanogpt.ImageGenerationBudget + filestore.MaxFetchDuration; writeTimeout <= image {
		t.Errorf("writeTimeout = %s, want it above an image call's %s", writeTimeout, image)
	}
}

func TestHealthz(t *testing.T) {
	srv := newTestServer(t)

	rec := httptest.NewRecorder()
	srv.router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 without a bearer token", rec.Code)
	}

	if rec.Body.String() != "ok" {
		t.Errorf("body = %q, want ok", rec.Body.String())
	}
}

func TestMCPRejectsBadCredentials(t *testing.T) {
	srv := newTestServer(t)

	authHeaders := map[string]string{
		"no header":    "",
		"wrong token":  "Bearer nope",
		"wrong scheme": "Basic server-key",
	}

	for name, authHeader := range authHeaders {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader("{}"))
			if authHeader != "" {
				req.Header.Set("Authorization", authHeader)
			}

			rec := httptest.NewRecorder()
			srv.router.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", rec.Code)
			}
		})
	}
}

func TestMCPAcceptsTheConfiguredToken(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer server-key")

	rec := httptest.NewRecorder()
	srv.router.ServeHTTP(rec, req)

	if rec.Code == http.StatusUnauthorized {
		t.Error("status = 401, want the request to reach the MCP handler")
	}
}
