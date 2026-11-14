package config

import (
	"errors"
	"fmt"
	"time"

	"github.com/VihangaFTW/catchup-mcp/internal/classroom"
)

const (
	envCenterFile = "CATCHUP_CENTER_FILE"
	envDemo       = "CATCHUP_DEMO"
	envMode       = "CATCHUP_MODE"
	envTimezone   = "CATCHUP_TIMEZONE"
)

type Config struct {
	Role     classroom.CourseRole
	Location *time.Location
	// CenterFile is the file path that stores client credentials.
	CenterFile string
	// use an in-memory center instead of a real Google client
	Demo bool
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
		return cfg, errors.New("student mode not supported yet, set CATCHUP_MODE=teacher")

	case classroom.RoleTeacher:

	default:
		return cfg, fmt.Errorf("unknown mode %q, set CATCHUP_MODE to teacher", cfg.Role)

	}

	if tz := getenv(envTimezone); tz != "" {
		loc, err := time.LoadLocation(tz)
		if err != nil {
			return cfg, fmt.Errorf("unknown timezone %q, use an IANA name such as Asia/Colombo: %w", tz, err)
		}
		cfg.Location = loc
	}

	if !cfg.Demo && cfg.CenterFile == "" {
		return cfg, errors.New("CATCHUP_CENTER_FILE is not set, so point it at a center.json file")
	}

	return cfg, nil
}
