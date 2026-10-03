package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wishmatic/nano-mcp/internal/auth"
	"github.com/wishmatic/nano-mcp/internal/config"
	"github.com/wishmatic/nano-mcp/internal/filestore"
	mcpServer "github.com/wishmatic/nano-mcp/internal/mcp"
	"github.com/wishmatic/nano-mcp/internal/nanogpt"
	"github.com/wishmatic/nano-mcp/internal/resolve"
	"github.com/wishmatic/nano-mcp/internal/sourcemap"
	"go.uber.org/zap"
)

// writeTimeout clears the longest call the client will make, so that call fails with the
// upstream error rather than a connection the server aborts mid-response. The longest is
// generate_video: waiting out a generation, then copying the result to disk.
const writeTimeout = nanogpt.VideoWaitBudget + filestore.MaxFetchDuration + time.Minute

type Server struct {
	cfg    config.Config
	log    *zap.Logger
	router *chi.Mux
	http   *http.Server
}

func New(cfg config.Config, log *zap.Logger) (*Server, error) {
	nano, err := nanogpt.New(nanogpt.Config{APIKey: cfg.NanoGPTAPIKey})
	if err != nil {
		return nil, err
	}

	return newServer(cfg, log, nano)
}

// newServer takes the client so tests can point it at a stub; New always builds one aimed at
// the real endpoint.
func newServer(cfg config.Config, log *zap.Logger, nano *nanogpt.Client) (*Server, error) {
	if cfg.APIKey == "" {
		return nil, auth.ErrNoAPIKey
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	publicBase, err := cfg.PublicBase()
	if err != nil {
		return nil, err
	}

	if publicBase == nil {
		return nil, fmt.Errorf("PUBLIC_HOST is required, so that videos have a URL")
	}

	files, err := filestore.New(filestore.Config{Dir: cfg.FilesDir, PublicBase: publicBase}, log)
	if err != nil {
		return nil, fmt.Errorf("configure file storage: %w", err)
	}

	sources, err := sourcemap.Parse(cfg.ImageURLMap)
	if err != nil {
		return nil, fmt.Errorf("IMAGE_URL_MAP: %w", err)
	}

	mcpSrv, err := mcpServer.New(mcpServer.Deps{
		Log:      log,
		NanoGPT:  nano,
		Files:    files,
		Resolver: resolve.New(sources),
	})
	if err != nil {
		return nil, fmt.Errorf("build mcp server: %w", err)
	}

	router := chi.NewRouter()

	router.Use(middleware.RequestID)
	router.Use(middleware.ClientIPFromRemoteAddr)
	router.Use(middleware.Recoverer)
	router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodOptions},
		AllowedHeaders:   []string{"Accept", "Content-Type", "Authorization"},
		ExposedHeaders:   []string{},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return mcpSrv
	}, nil)

	files.Register(router)

	log.Info("video hosting enabled",
		zap.String("dir", cfg.FilesDir),
		zap.String("public_host", cfg.PublicHost),
	)
	log.Warn("stored videos are readable by anyone with the URL")

	router.Mount("/mcp", auth.Middleware(log, cfg.APIKey)(handler))

	router.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	return &Server{
		cfg:    cfg,
		log:    log,
		router: router,
		http: &http.Server{
			Addr:              cfg.Addr(),
			Handler:           router,
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      writeTimeout,
			IdleTimeout:       60 * time.Second,
		},
	}, nil
}

func (s *Server) Run() error {
	s.log.Info("server listening", zap.String("addr", s.cfg.Addr()))

	if err := s.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}
