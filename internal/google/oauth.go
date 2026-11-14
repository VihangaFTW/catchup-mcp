package google

import "golang.org/x/oauth2"

const scopePrefix = "https://www.googleapis.com/auth/"

var TeacherScopes = []string{
	scopePrefix + "classroom.courses.readonly",
	scopePrefix + "classroom.topics.readonly",
	scopePrefix + "classroom.announcements.readonly",
	scopePrefix + "classroom.courseworkmaterials.readonly",
	scopePrefix + "classroom.coursework.students.readonly",
	scopePrefix + "classroom.rosters.readonly",
}

var OAuthEndpoint = oauth2.Endpoint{
	AuthURL:   "https://accounts.google.com/o/oauth2/auth",
	TokenURL:  "https://oauth2.googleapis.com/token",
	AuthStyle: oauth2.AuthStyleInParams,
}
