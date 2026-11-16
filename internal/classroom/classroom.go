package classroom

import (
	"context"
	"fmt"
)

type CourseRole string

const (
	RoleTeacher CourseRole = "teacher"
	RoleStudent CourseRole = "student"
)

func (r CourseRole) Other() CourseRole {
	if r == RoleTeacher {
		return RoleStudent
	}

	return RoleTeacher
}

type CourseState string

const (
	StateActive   CourseState = "ACTIVE"
	StateArchived CourseState = "ARCHIVED"
)

type Course struct {
	ID      string
	Name    string
	Section string
	// DescriptionHeading is the title shown above the description
	// on the "About" page of the course.
	DescriptionHeading string
	Description        string
	Level              string
	Subject            string
	Room               string
	State              CourseState
}

// Label returns a human readable identifier for a Course.
func (c Course) Label() string {
	label := c.Name

	if c.Section != "" {
		label += ", " + c.Section
	}

	if c.State == StateArchived {
		label += " [archived]"
	}

	return fmt.Sprintf("%s (id %s)", label, c.ID)
}

func (c Course) Meta() string {
	meta := c.Name

	if c.Section != "" {
		meta += ", " + c.Section
	}

	switch {
	case c.DescriptionHeading != "" && c.Description != "":
		meta += " - " + c.DescriptionHeading + ": " + c.Description
	case c.DescriptionHeading != "":
		meta += " - " + c.DescriptionHeading
	case c.Description != "":
		meta += " - " + c.Description
	}

	if c.Level != "" {
		meta += " [level: " + c.Level + "]"
	}

	if c.Subject != "" {
		meta += " [subject: " + c.Subject + "]"
	}

	if c.Room != "" {
		meta += " [room: " + c.Room + "]"
	}

	if c.State == StateArchived {
		meta += " [archived]"
	}

	return meta
}

type Classroom interface {
	ListCourses(ctx context.Context, role CourseRole, states []CourseState) ([]Course, error)
}
