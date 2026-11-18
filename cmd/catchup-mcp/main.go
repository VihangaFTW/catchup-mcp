package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/VihangaFTW/catchup-mcp/internal/auth"
	"github.com/VihangaFTW/catchup-mcp/internal/classroom"
	"github.com/VihangaFTW/catchup-mcp/internal/config"
	"github.com/VihangaFTW/catchup-mcp/internal/google"
	"github.com/VihangaFTW/catchup-mcp/internal/logging"
	"github.com/VihangaFTW/catchup-mcp/internal/server"
	"golang.org/x/oauth2"
)

func main() {

	slog.SetDefault(logging.NewLogger(os.Stderr, slog.LevelInfo, logging.FormatText))

	cfg, err := config.Load(os.Getenv)

	if err != nil {
		fatal("cannot load the configuration", err)
	}

	// reset the default logger with cfg parameters
	slog.SetDefault(logging.NewLogger(os.Stderr, cfg.LogLevel, cfg.LogFormat))

	slog.Info("catchup starts",
		"version", server.Version,
		"role", cfg.Role,
		"timezone", cfg.Location.String(),
		"demo", cfg.Demo,
		"log_level", cfg.LogLevel.String(),
		"log_format", cfg.LogFormat,
	)

	if !cfg.Demo {
		slog.Info("catchup uses center file", "path", cfg.CenterFile)
	}

	cls, err := newClassroom(cfg)

	if err != nil {
		fatal("cannot set up the Classroom client", err)
	}

	tk := server.Toolkit{
		Config:    cfg,
		Classroom: cls,
		Now:       time.Now,
	}

	// intercept Interrupt and SIGTERM signals to this process
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	httpSrv := server.NewHTTPServer(&tk)

	// http server runs in a separate goroutine from the main thread
	// channel allows us to handle case when the server errors
	serveErr := make(chan error, 1)

	go func() {
		serveErr <- httpSrv.ListenAndServe()
	}()

	slog.Info("catchup serves MCP", "url", "http://"+server.ListenAddr+server.MCPPath)

	select {
	// the server died
	case err := <-serveErr:
		fatal("the mcp server stopped", err)
	// OS signal aimed at process
	case <-ctx.Done():
		slog.Info("catchup received a stop signal and shuts down")
		// gracefully shutdown server
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		if err := httpSrv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fatal("the MCP server did not shut down cleanly", err)
		}
	}

	slog.Info("catchup stopped")

}

func newClassroom(cfg config.Config) (classroom.Classroom, error) {

	if cfg.Demo {
		return nil, errors.New("demo mode is not implemented yet")
	}

	center, err := auth.LoadClientCreds(cfg.CenterFile)

	if err != nil {
		return nil, err
	}

	tokenPath, err := auth.DefaultTokenPath(string(cfg.Role))

	if err != nil {
		return nil, err
	}

	slog.Debug("sign-in storage is set", "center_file", cfg.CenterFile, "token_file", tokenPath)

	a := auth.New(&oauth2.Config{
		ClientID:     center.ClientID,
		ClientSecret: center.ClientSecret,
		Endpoint:     google.OAuthEndpoint,
		Scopes:       google.TeacherScopes,
	}, auth.FileTokenStore{Path: tokenPath}, auth.OpenBrowser)

	return google.NewClassroom(a.HTTPClient, a.Reauthorize), nil

}

// fatal logs err at the Error level and stops the process with exit code 1.
// Deferred functions do not run, so call it only from main.
func fatal(msg string, err error) {
	slog.Error(msg, "err", err)
	os.Exit(1)
}
