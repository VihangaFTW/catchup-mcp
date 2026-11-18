package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/VihangaFTW/catchup-mcp/internal/classroom"
	"github.com/VihangaFTW/catchup-mcp/internal/logging"
)

const (
	envCenterFile = "CATCHUP_CENTER_FILE"
	envDemo       = "CATCHUP_DEMO"
	envMode       = "CATCHUP_MODE"
	envTimezone   = "CATCHUP_TIMEZONE"
	envLogLevel   = "CATCHUP_LOG_LEVEL"
	envLogFormat  = "CATCHUP_LOG_FORMAT"
)

const (
	appDir     = "catchup"
	fileCenter = "center.json"
)

type Config struct {
	Role     classroom.CourseRole
	Location *time.Location
	// CenterFile is the file path that stores client credentials.
	CenterFile string
	// use an in-memory center instead of a real Google client
	Demo bool
	// LogLevel is the minimum reporting log level.
	LogLevel slog.Level
	// LogFormat is the serialization format used for log outputs. Supported formats are "text" and "json".
	LogFormat logging.Format
}

func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		Location:   time.Local,
		CenterFile: getenv(envCenterFile),
		Role:       classroom.CourseRole(getenv(envMode)),
		Demo:       getenv(envDemo) == "1",
	}

	if cfg.Role == "" {
		cfg.Role = classroom.RoleTeacher
	}

	// refactor: only teacher mode supported in v1
	switch cfg.Role {

	case classroom.RoleStudent:
		return cfg, fmt.Errorf("student mode not supported yet, set %s=teacher", envMode)

	case classroom.RoleTeacher:

	default:
		return cfg, fmt.Errorf("unknown mode %q, set %s to teacher", cfg.Role, envMode)

	}

	if tz := getenv(envTimezone); tz != "" {
		loc, err := time.LoadLocation(tz)
		if err != nil {
			return cfg, fmt.Errorf("unknown timezone %q, use an IANA name such as Asia/Colombo: %w", tz, err)
		}
		cfg.Location = loc
	}

	if !cfg.Demo && cfg.CenterFile == "" {
		// fallback to the center file in the user config directory, next to the tokens
		dir, err := os.UserConfigDir()
		if err != nil {
			return cfg, fmt.Errorf("%s is not set and the config directory cannot be found: %w", envCenterFile, err)
		}

		path := filepath.Join(dir, appDir, fileCenter)

		if _, err := os.Stat(path); err != nil {
			return cfg, fmt.Errorf("%s is not set and the fallback center file cannot be used: %w", envCenterFile, err)
		}

		cfg.CenterFile = path
	}

	ll, err := logging.ParseLevel(getenv(envLogLevel))

	if err != nil {
		return cfg, fmt.Errorf("%w, set %s to debug, info, warn or error", err, envLogLevel)
	}

	cfg.LogLevel = ll

	lf, err := logging.ParseFormat(getenv(envLogFormat))

	if err != nil {
		return cfg, fmt.Errorf("%w, set %s to text or json", err, envLogFormat)
	}

	cfg.LogFormat = lf

	return cfg, nil
}
