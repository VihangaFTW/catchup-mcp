package google

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/VihangaFTW/catchup-mcp/internal/classroom"
)

const classroomBaseURL = "https://classroom.googleapis.com"

const requestTimeout = 30 * time.Second

const (
	reasonScopeInsufficient = "ACCESS_TOKEN_SCOPE_INSUFFICIENT"
	reasonServiceDisabled   = "SERVICE_DISABLED"
)

type Classroom struct {
	baseURL string
	// client returns the HTTP client used to send requests to the Classroom API.
	// It returns an error when a client cannot be built.
	client func(context.Context) (*http.Client, error)
	// reauthorize throws away the saved sign-in and starts a new one.
	reauthorize func() error
}

func NewClassroom(client func(context.Context) (*http.Client, error), reauthorize func() error) *Classroom {
	return &Classroom{
		baseURL:     classroomBaseURL,
		client:      client,
		reauthorize: reauthorize,
	}
}

// googleErrorBody holds the parts of a Google API error response.
// It is decoded from the JSON body of a failed request.
type googleErrorBody struct {
	Error struct {
		Message string `json:"message"`
		Status  string `json:"status"`
		// details holds entries of several kinds. Only the ErrorInfo entry has a reason.
		Details []struct {
			Reason string `json:"reason"`
		} `json:"details"`
	} `json:"error"`
}

// reason returns the most exact cause that Google gave.
func (eb googleErrorBody) reason() string {
	for _, d := range eb.Error.Details {
		if r := d.Reason; r != "" {
			return r
		}
	}

	return eb.Error.Status
}

// apiError is a failed Classroom API response, reduced to the three values
// that the rest of the program uses.
type apiError struct {
	// code is the HTTP status code.
	code int
	// reason is the exact cause from Google's ErrorInfo entry. It is empty
	// when the body has none.
	reason string
	// message is Google's own description. It is empty when the body has none.
	message string
}

// newAPIError reads Google's error body from resp. A body that is missing or
// is not JSON gives an apiError that holds only the HTTP status.
func newAPIError(resp *http.Response) *apiError {
	apiErr := &apiError{
		code: resp.StatusCode,
	}

	var body googleErrorBody

	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		if !errors.Is(err, io.EOF) {
			slog.Warn("cannot read the error body of a failed response", "status", apiErr.code, "err", err)
		}
		return apiErr
	}

	apiErr.reason = body.reason()
	apiErr.message = body.Error.Message

	return apiErr
}

func (e *apiError) Error() string {
	s := fmt.Sprintf("google classroom api: http %d %s", e.code, http.StatusText(e.code))

	if e.message != "" {
		s += ": " + e.message
	}

	return s
}

func (e *apiError) rejectsSignIn() bool {
	return e.code == http.StatusUnauthorized || e.reason == reasonScopeInsufficient
}

// advice tells the user what to do about the error. It ends with Google's own
// message when the response had one.
func (e *apiError) advice() string {
	var adv string

	switch {
	case e.rejectsSignIn():
		adv = "Google rejected the saved sign-in. Ask again to sign in once more."
	case e.reason == reasonServiceDisabled:
		adv = "The Classroom API is not enabled for"

	case e.code == http.StatusForbidden:
		adv = "Google denied access."

	case e.code == http.StatusTooManyRequests:
		adv = "Google is limiting requests. Wait a minute and ask again."

	default:
		adv = fmt.Sprintf("Google returned http %d. Try again later.", e.code)
	}

	if e.message != "" {
		adv += " Google said: " + e.message
	}

	return adv
}

func (gc *Classroom) ListCourses(ctx context.Context, role classroom.CourseRole, states []classroom.CourseState) ([]classroom.Course, error) {
	queries := url.Values{}

	switch role {
	case classroom.RoleTeacher:
		queries.Set("teacherId", "me")
	case classroom.RoleStudent:
		queries.Set("studentId", "me")
	}

	for _, s := range states {
		queries.Add("courseStates", string(s))
	}

	// max entries
	queries.Set("pageSize", "100")

	var courses []classroom.Course

	for {
		var page struct {
			Courses []struct {
				ID                 string `json:"id"`
				Name               string `json:"name"`
				Section            string `json:"section"`
				DescriptionHeading string `json:"descriptionHeading"`
				Description        string `json:"description"`
				Levels             string `json:"levels"`
				Subject            string `json:"subject"`
				Room               string `json:"room"`
				CourseState        string `json:"courseState"`
			} `json:"courses"`
			NextPageToken string `json:"nextPageToken"`
		}

		if err := gc.get(ctx, "/v1/courses", queries, &page); err != nil {
			return nil, err
		}

		for _, c := range page.Courses {
			courses = append(courses, classroom.Course{
				ID:                 c.ID,
				Name:               c.Name,
				Section:            c.Section,
				DescriptionHeading: c.DescriptionHeading,
				Description:        c.Description,
				Level:              c.Levels,
				Subject:            c.Subject,
				Room:               c.Room,
				State:              classroom.CourseState(c.CourseState),
			})
		}

		if page.NextPageToken == "" {
			return courses, nil
		}
		queries.Set("pageToken", page.NextPageToken)

	}

}

func (gc *Classroom) get(ctx context.Context, path string, q url.Values, v any) error {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	client, err := gc.client(ctx)

	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, gc.baseURL+path+"?"+q.Encode(), nil)

	if err != nil {
		return err
	}

	// Do is blocking until response arrives.
	// Internally, it waits on a channel that returns the response.
	resp, err := client.Do(req)

	if err != nil {
		return err
	}

	// release connection once Do() returns
	defer resp.Body.Close()

	// wrap error response in apiError struct
	if resp.StatusCode != http.StatusOK {
		apiErr := newAPIError(resp)

		// check if re authorization is possible
		if gc.reauthorize != nil && apiErr.rejectsSignIn() {
			return gc.reauthorize()
		}

		return apiErr

	}

	return json.NewDecoder(resp.Body).Decode(v)
}
