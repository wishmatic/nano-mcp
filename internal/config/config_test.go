package config

import (
	"strings"
	"testing"
)

func TestAddr(t *testing.T) {
	cfg := Config{Host: "127.0.0.1", Port: 9000}

	if got := cfg.Addr(); got != "127.0.0.1:9000" {
		t.Errorf("Addr() = %q, want 127.0.0.1:9000", got)
	}
}

func TestValidateAcceptsThePortBounds(t *testing.T) {
	for _, port := range []int{1, 65535} {
		cfg := Config{Port: port}

		if err := cfg.Validate(); err != nil {
			t.Errorf("Validate() error for port %d = %v, want it accepted", port, err)
		}
	}
}

func TestValidateRejectsOutOfRangePorts(t *testing.T) {
	for _, port := range []int{0, -1, 65536} {
		cfg := Config{Port: port}

		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), "PORT") {
			t.Errorf("Validate() error for port %d = %v, want it to name PORT", port, err)
		}
	}
}

func TestPublicBase(t *testing.T) {
	tests := map[string]struct {
		publicHost string
		want       string
		wantErr    bool
	}{
		"unset":             {want: ""},
		"host":              {publicHost: "https://nano.example.com", want: "https://nano.example.com"},
		"trailing slash":    {publicHost: "https://nano.example.com/", want: "https://nano.example.com"},
		"no scheme":         {publicHost: "nano.example.com", wantErr: true},
		"no host":           {publicHost: "https://", wantErr: true},
		"a path":            {publicHost: "https://nano.example.com/files", wantErr: true},
		"a query":           {publicHost: "https://nano.example.com?a=b", wantErr: true},
		"not a URL":         {publicHost: "://nano", wantErr: true},
		"a non-http scheme": {publicHost: "ftp://nano.example.com", wantErr: true},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			base, err := Config{PublicHost: tt.publicHost}.PublicBase()
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "PUBLIC_HOST") {
					t.Fatalf("PublicBase() error = %v, want it to name PUBLIC_HOST", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("PublicBase() error: %v", err)
			}

			if tt.want == "" {
				if base != nil {
					t.Errorf("PublicBase() = %v, want nil when PUBLIC_HOST is unset", base)
				}

				return
			}

			if base == nil || base.String() != tt.want {
				t.Errorf("PublicBase() = %v, want %q", base, tt.want)
			}
		})
	}
}

func TestLoadReadsTheEnvironment(t *testing.T) {
	t.Setenv("HOST", "127.0.0.1")
	t.Setenv("PORT", "9100")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("API_KEY", "secret")
	t.Setenv("NANOGPT_API_KEY", "nano-secret")
	t.Setenv("IMAGE_URL_MAP", "https://chat.example.com/images/=/data/images")
	t.Setenv("PUBLIC_HOST", "https://nano.example.com")
	t.Setenv("FILES_DIR", "/var/lib/nano-mcp")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Addr() != "127.0.0.1:9100" {
		t.Errorf("Addr() = %q, want 127.0.0.1:9100", cfg.Addr())
	}

	if cfg.LogLevel != "debug" || cfg.APIKey != "secret" {
		t.Errorf("Load() = %+v, want the configured level and key", cfg)
	}

	if cfg.NanoGPTAPIKey != "nano-secret" {
		t.Errorf("Load() = %+v, want the configured nano-gpt key", cfg)
	}

	if cfg.PublicHost != "https://nano.example.com" || cfg.FilesDir != "/var/lib/nano-mcp" {
		t.Errorf("Load() = %+v, want the configured public host and files directory", cfg)
	}

	if cfg.ImageURLMap != "https://chat.example.com/images/=/data/images" {
		t.Errorf("Load() = %+v, want the configured address map", cfg)
	}
}

func TestLoadAppliesTheDefaults(t *testing.T) {
	t.Setenv("HOST", "")
	t.Setenv("PORT", "")
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("API_KEY", "")
	t.Setenv("NANOGPT_API_KEY", "")
	t.Setenv("PUBLIC_HOST", "")
	t.Setenv("FILES_DIR", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Host != "0.0.0.0" || cfg.Port != 8080 || cfg.LogLevel != "info" || cfg.APIKey != "" {
		t.Errorf("Load() = %+v, want the documented defaults", cfg)
	}

	if cfg.FilesDir != "files" || cfg.PublicHost != "" {
		t.Errorf("Load() = %+v, want the files default with no public host", cfg)
	}
}

func TestLoadRejectsANonNumericPort(t *testing.T) {
	t.Setenv("PORT", "http")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want the malformed port rejected")
	}
}
