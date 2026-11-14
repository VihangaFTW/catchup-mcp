package server

import (
	"fmt"
	"time"
)


type Clock func() time.Time		

func today(now time.Time, loc *time.Location) string {
	t := now.In(loc)
	zone := loc.String()

	if zone == "" || zone == "Local" {
		zone, _ = t.Zone()
	}

	// examples: "Today: 2026-11-07 (Asia/Colombo)"
	// "Today: 2026-11-07 (+0530)" for Local
	return fmt.Sprintf("Today: %s (%s)", t.Format(time.DateOnly), zone)
}
