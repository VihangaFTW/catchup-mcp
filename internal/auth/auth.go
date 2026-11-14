package auth

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/oauth2"
)

const (
	// owner only with list, create and enter permissions
	privateDirPerm fs.FileMode = 0o700
	// owner only with read and write perms
	privateFilePerm fs.FileMode = 0o600
)

// ClientCreds stores the credentials for a Google OAuth client.
// Such a client can be created in a Google Cloud project.
type ClientCreds struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

func LoadClientCreds(path string) (ClientCreds, error) {
	var cf ClientCreds

	bytes, err := os.ReadFile(path)

	if err != nil {
		return cf, fmt.Errorf("cannot read center file %s: %w", path, err)
	}

	if err := json.Unmarshal(bytes, &cf); err != nil {
		return cf, fmt.Errorf("the center file %s is not valid JSON: %w", path, err)
	}

	if cf.ClientID == "" {
		return cf, fmt.Errorf("the center file %s has no client_id", path)
	}

	if cf.ClientSecret == "" {
		return cf, fmt.Errorf("the center file %s has no client_secret", path)
	}

	return cf, nil
}

// SignInStartedError reports that no usable token exists and a browser
// sign-in has started.
type SignInStartedError struct {
	// URL is the authorization server's consent page.
	URL string
}

func (e *SignInStartedError) Error() string {
	return "sign-in required: " + e.URL
}

// TokenMeta holds an oauth2 token along with its
// metadata such as its scopes and associated clientId.
type TokenMeta struct {
	ClientID string        `json:"client_id"`
	Scopes   []string      `json:"scopes"`
	Token    *oauth2.Token `json:"token"`
}

type tokenSource struct {
	// src holds the oauth2 token source. It's Token() method
	// returns the current access token or a new token if it's expired.
	src oauth2.TokenSource
	// saveToken saves a token to disk.
	saveToken func(*oauth2.Token) error
	// mu guards the `lastToken` field.
	mu sync.Mutex
	// lastToken refers to the access token string from the most recent save.
	lastToken string
}

func (ts *tokenSource) Token() (*oauth2.Token, error) {
	latest, err := ts.src.Token()

	if err != nil {
		return nil, err
	}

	// update lastToken if token has been refreshed
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if ts.lastToken != latest.AccessToken {

		// a failed save must not fail the request: latest is still a valid token.
		// lastToken stays unchanged, so the next call tries the save again.
		if err := ts.saveToken(latest); err != nil {
			slog.Warn("cannot save refreshed oauth2 token", "err", err)
		} else {
			ts.lastToken = latest.AccessToken
		}
	}

	return latest, nil

}

// ErrBadTokenFile reports that the saved sign-in exists but cannot be used:
// it is not valid JSON or it has no token. A new sign-in replaces it.
var ErrBadTokenFile = errors.New("saved sign-in is damaged")

type TokenStore interface {
	// Load returns fs.ErrNotExist when nothing is saved and
	// ErrBadTokenFile when the saved data cannot be used.
	Load() (*TokenMeta, error)
	Save(*TokenMeta) error
	Delete() error
}

type FileTokenStore struct {
	Path string
}

func DefaultTokenPath(role string) (string, error) {
	dir, err := os.UserConfigDir()

	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "catchup", "token-"+role+".json"), nil
}

func (s FileTokenStore) Load() (*TokenMeta, error) {
	bytes, err := os.ReadFile(s.Path)

	if err != nil {
		return nil, err
	}

	var si TokenMeta

	if err := json.Unmarshal(bytes, &si); err != nil {
		return nil, fmt.Errorf("%w: %s is not valid JSON: %w", ErrBadTokenFile, s.Path, err)
	}

	if si.Token == nil {
		return nil, fmt.Errorf("%w: %s has no token", ErrBadTokenFile, s.Path)
	}

	return &si, nil

}

// Save writes the saved sign-in struct securely to a private json file.
//
// The token is written to a temporary file and then renamed instead of calling
// WriteFile() directly to avoid half written files in the event of the process crashing.
func (s FileTokenStore) Save(si *TokenMeta) error {
	if err := os.MkdirAll(filepath.Dir(s.Path), privateDirPerm); err != nil {
		return err
	}

	bytes, err := json.Marshal(si)

	if err != nil {
		return err
	}

	// temp file created with mode 0600 by default
	// here, tmp.Name() returns a path
	tmp, err := os.CreateTemp(filepath.Dir(s.Path), ".signin-*")

	if err != nil {
		return err
	}

	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(bytes); err != nil {
		tmp.Close()
		return err
	}

	// Sync() calls the system call fsync under the hood
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}

	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmp.Name(), s.Path)

}

func (s FileTokenStore) Delete() error {
	err := os.Remove(s.Path)

	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}

	return err
}

// Auth holds the required oauth2 configuration for every MCP tool call.
type Auth struct {
	cfg   *oauth2.Config
	store TokenStore
	// onStateChange runs a callback that executes some outside business logic
	// related to Auth's dependencies whenever changes are made to the signed-in user state.
	//
	// Example usage: clearing a cache when a new sign in token is generated.
	onStateChange func()
	// openURL opens the authorization server's consent page in the user’s default browser.
	openURL func(string) error
	// timeout holds the max duration allocated for the sign-in process.
	timeout time.Duration
	// mu checks for an existing sign in flow to prevent race conditions
	mu sync.Mutex
	// field access must be guarded by a mutex
	pending *pendingSignIn
}

type pendingSignIn struct {
	url string
	srv *http.Server
	// timer acts as a timeout for cases where user does not finish the sign-in process.
	timer *time.Timer
}

func New(cfg *oauth2.Config, store TokenStore, openURL func(string) error) *Auth {
	return &Auth{
		cfg:     cfg,
		store:   store,
		openURL: openURL,
		timeout: 5 * time.Minute,
	}
}

// HTTPClient returns an HTTP client that is authorized with the saved token.
//
// If no usable token exists, it starts a browser sign-in and returns a
// *SignInStartedError.
func (a *Auth) HTTPClient(ctx context.Context) (*http.Client, error) {
	saved, err := a.store.Load()

	if errors.Is(err, fs.ErrNotExist) {
		return nil, a.signIn()
	}

	// a damaged file cannot be repaired, so replace it with a new sign-in
	if errors.Is(err, ErrBadTokenFile) {
		slog.Warn("replacing the saved sign-in", "cause", err)
		return nil, a.Reauthorize()
	}

	if err != nil {
		return nil, err
	}

	// saved token exists, check if user owns it
	if !saved.matches(a.cfg) {
		return nil, a.Reauthorize()
	}

	src := &tokenSource{
		src:       a.cfg.TokenSource(ctx, saved.Token),
		lastToken: saved.Token.AccessToken,
		saveToken: a.saveToken,
	}

	// handle cases: user revoked access, the token expired, or it was replaced
	// via a new sign in flow. Other errors are returned.
	if _, err := src.Token(); err != nil {
		if re, ok := errors.AsType[*oauth2.RetrieveError](err); ok && re.ErrorCode == "invalid_grant" {
			return nil, a.Reauthorize()
		}

		return nil, err
	}

	return oauth2.NewClient(ctx, src), nil

}

// Reauthorize deletes the saved token and starts a new browser sign-in.
//
// It always returns a non-nil error. A *SignInStartedError means the flow
// started. Any other error means the token could not be deleted or the flow
// could not start.
func (a *Auth) Reauthorize() error {
	if err := a.store.Delete(); err != nil {
		return err
	}

	if a.onStateChange != nil {
		a.onStateChange()
	}

	return a.signIn()
}

// saveToken persists a fresh oauth2 token to disk
// so that it can be reused for future sessions.
func (a *Auth) saveToken(token *oauth2.Token) error {
	return a.store.Save(&TokenMeta{
		ClientID: a.cfg.ClientID,
		Scopes:   a.cfg.Scopes,
		Token:    token,
	})
}

// matches reports whether the saved sign-in token was made with the same OAuth
// client and covers every scope that cfg asks for. A false result means the token
// cannot be used and a new sign-in is necessary.
func (tm *TokenMeta) matches(cfg *oauth2.Config) bool {
	return tm.Token != nil && tm.ClientID == cfg.ClientID && len(missingScopes(cfg.Scopes, tm.Scopes)) == 0
}

// signIn starts a browser sign-in, or joins the one already in progress.
//
// It always returns a non-nil error. A *SignInStartedError means the flow
// started. Any other error means the flow could not start.
func (a *Auth) signIn() error {
	url, err := a.startSignIn()

	if err != nil {
		return fmt.Errorf("cannot start sign-in: %w", err)
	}

	return &SignInStartedError{URL: url}
}

func (a *Auth) startSignIn() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.pending != nil {
		return a.pending.url, nil
	}

	// 0 assigns a random available ephemeral port
	listener, err := net.Listen("tcp", "127.0.0.1:0")

	if err != nil {
		return "", err
	}

	cfg := *a.cfg
	// define local endpoint that communicates with user's browser
	cfg.RedirectURL = "http://" + listener.Addr().String() + "/callback"

	// Generate two secure random strings known only by this process
	// for URL that leads to Google's consent page.

	// state ensures the redirect came from the sign in flow that started it.
	// It is added as a URL query "state".
	state := oauth2.GenerateVerifier()
	// verfifier is hashed before added as a URL query "code_challenge".
	// After google sends back a code, the process sends back the verifier
	// to prove the challenge was sent by this process.
	verifier := oauth2.GenerateVerifier()

	pending := &pendingSignIn{
		url: cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce, oauth2.S256ChallengeOption(verifier)),
	}

	// The authorization code works only once, so only the first valid callback is processed.
	//
	// HTTP requests are handled in separate goroutines so a plain bool cannot safely be read
	// and written from two goroutines at once.
	var used atomic.Bool

	mux := http.NewServeMux()
	mux.HandleFunc("GET /callback", func(w http.ResponseWriter, r *http.Request) {
		// prevent caching one time code in client's memory or disk
		w.Header().Set("Cache-Control", "no-store")
		q := r.URL.Query()

		// guard against an improbable timing attack: compare every byte with our state
		// instead of a != string comparion, which stops at the first mismatch and can
		// theoretically be abused with multiple requests under a narrow window to guess state
		if subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(state)) != 1 {
			http.Error(w, "The sign-in link is not valid. Close this tab and ask again.", http.StatusBadRequest)
			return
		}

		// check if code has been used before; if yes, swap
		if !used.CompareAndSwap(false, true) {
			http.Error(w, "This sign-in has already finished. Close this tab.", http.StatusGone)
			return
		}

		// shutdown callback server when handler returns from this point onwards
		defer a.finish(pending)

		if e := q.Get("error"); e != "" {
			http.Error(w, fmt.Sprintf("Sign-in did not finish because Google returned %q. Close this tab and ask again.", e), http.StatusBadRequest)
			return
		}

		code := q.Get("code")

		if code == "" {
			http.Error(w, "Google did not send a sign-in code.Close this tab and ask again.", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		// get oauth token from Google
		// the scopes the user accepted come back in the token's "scope" extra field
		token, err := cfg.Exchange(ctx, code, oauth2.VerifierOption(verifier))

		if err != nil {
			slog.Error("oauth2 code exchange failed", "err", err)
			http.Error(w, "Google rejected the sign-in. Close this tab and ask again.", http.StatusBadRequest)

			return
		}

		// check token scopes as user can deny scopes in consent screen
		// granted is one string of user-accepted scopes separated by whitespace
		if granted, ok := token.Extra("scope").(string); ok {
			if missing := missingScopes(cfg.Scopes, strings.Fields(granted)); len(missing) > 0 {
				slog.Warn("sign-in lacks scopes", "missing", missing)
				http.Error(w, "Some permissions were denied, so the sign-in was not saved. Close this tab, ask again and accept all permissions.", http.StatusForbidden)
				return
			}
		}

		// save token to disk
		if err := a.saveToken(token); err != nil {
			slog.Error("cannot save oauth2 token", "err", err)
			http.Error(w, "The sign-in worked but could not be saved. Close this tab and ask again.", http.StatusInternalServerError)
			return
		}

		// execute cleanup logic post state change
		if a.onStateChange != nil {
			a.onStateChange()
		}

		// write success message in plaintext in response body
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		// "nosniff" instructs the browser to render the body strictly as plaintext
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "Signed in successfully.")

	})

	pending.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	// shutdown server on timeout
	pending.timer = time.AfterFunc(a.timeout, func() { a.finish(pending) })

	a.pending = pending

	go func() {
		if err := pending.srv.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
			slog.Error("sign-in callback server stopped", "err", err)
		}
	}()

	if err := a.openURL(pending.url); err != nil {
		slog.Warn("cannot open the browser, use the link instead", "err", err)
	}

	return pending.url, nil
}

var scopeAliases = map[string]string{
	"https://www.googleapis.com/auth/classroom.student-submissions.students.readonly": "https://www.googleapis.com/auth/classroom.coursework.students.readonly",
	"https://www.googleapis.com/auth/classroom.student-submissions.me.readonly":       "https://www.googleapis.com/auth/classroom.coursework.me.readonly",
}

func missingScopes(want, got []string) []string {
	canonical := make([]string, len(got))

	for i, scp := range got {
		if alias, ok := scopeAliases[scp]; ok {
			scp = alias
		}
		canonical[i] = scp
	}

	var missing []string

	for _, scp := range want {
		if !slices.Contains(canonical, scp) {
			missing = append(missing, scp)
		}
	}
	return missing
}

// finish ends the sign-in attempt p: it clears a's pending field if p is still
// the current attempt, stops p's timer and shuts down p's callback server.
//
// Both the callback handler (deferred) and the timeout timer call it.
// A second call for the same p is safe.
func (a *Auth) finish(p *pendingSignIn) {
	a.mu.Lock()

	if a.pending == p {
		a.pending = nil
	}

	p.timer.Stop()
	a.mu.Unlock()

	// shutdown server in a goroutine to avoid blocking on the handler that calls this method
	go func() {
		// shutdown call can wait forever, so giving this thread a timeout
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := p.srv.Shutdown(ctx); err != nil {
			// drops all remaining (stuck) connections immediately
			p.srv.Close()
		}

	}()

}

// OpenBrowser opens url in the host's default browser.
func OpenBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	// macOS
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	// linux
	default:
		name := "xdg-open"

		// wsl
		if _, err := exec.LookPath("wslview"); err == nil {
			name = "wslview"
		}

		cmd = exec.Command(name, url)

	}

	if err := cmd.Start(); err != nil {
		return err
	}

	// collect the exit status so the finished opener does not stay as a zombie process
	go cmd.Wait()

	return nil
}
