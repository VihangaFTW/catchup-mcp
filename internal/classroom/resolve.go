package classroom

import (
	"fmt"
	"strings"
)

func MatchCourses(courses []Course, query string) []Course {
	query = strings.TrimSpace(query)

	// query matches a course ID
	for i := range courses {
		if courses[i].ID == query {
			return []Course{courses[i]}
		}
	}

	// keyword search for matching courses
	keywords := strings.Fields(strings.ToLower(query))

	var matches []Course

	for _, c := range courses {
		haystack := strings.ToLower(c.Meta())

		if containsAll(haystack, keywords) {
			matches = append(matches, c)
		}
	}

	return matches
}

func containsAll(haystack string, keywords []string) bool {
	for _, t := range keywords {
		if !strings.Contains(haystack, t) {
			return false
		}
	}
	return true
}

func ResolutionMsg(query string, matches, scope []Course) string {
	var b strings.Builder

	list := matches

	if len(matches) > 1 {
		fmt.Fprintf(&b, "Several courses match %q. Ask the user which ones they mean. \n ", query)
	} else {
		// shows all available courses when nothing matches
		fmt.Fprintf(&b, "No classes matches %q. These are the classes in scope.\n", query)
		list = scope

	}

	for _, c := range list {
		fmt.Fprintf(&b, "- %s\n", c.Label())
	}

	return b.String()
}
