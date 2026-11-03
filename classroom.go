package main

import (
	"context"
	"fmt"
)

type CourseRole string

const (
	RoleTeacher CourseRole = "teacher"
	RoleStudent CourseRole = "student"
)

type ClassState string

const (
	StateActive   ClassState = "ACTIVE"
	StateArchived ClassState = "ARCHIVED"
)

type Course struct {
	ID          string
	Name        string
	Section     string
	Description string
	Level       string
	Subject     string
	Room        string
	State       ClassState
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

	if c.Description != "" {
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
	ListCourses(ctx context.Context, role CourseRole, states []ClassState) ([]Course, error)
}
