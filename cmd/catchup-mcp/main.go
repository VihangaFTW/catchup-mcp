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
	"github.com/VihangaFTW/catchup-mcp/internal/server"
	"golang.org/x/oauth2"
)

func main() {

	cfg, err := config.Load(os.Getenv)

	if err != nil {
		fatal("cannot load the configuration", err)
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
		// gracefully shutdown server
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		if err := httpSrv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fatal("the MCP server did not shut down cleanly", err)
		}
	}

}

func newClassroom(cfg config.Config) (classroom.Classroom, error) {
	if cfg.Demo {
		fatal("Demo mode has not been implemented", nil)
	}

	center, err := auth.LoadClientCreds(cfg.CenterFile)

	if err != nil {
		return nil, err
	}

	tokenPath, err := auth.DefaultTokenPath(string(cfg.Role))

	if err != nil {
		return nil, err
	}

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
