// Package session defines the domain object shared across packages.
package session

import "time"

// Session is one Claude Code chat as displayed in ccs.
type Session struct {
	UUID         string
	ProjectPath  string
	Cwd          string
	GitBranch    string
	Title        string
	LastActivity time.Time
	MsgCount     int
	Mtime        time.Time
	FilePath     string

	Pinned      bool
	Tags        []string
	Hidden      bool
	CustomTitle string
	Notes       string

	Missing bool
}

func (s Session) DisplayTitle() string {
	if s.CustomTitle != "" {
		return s.CustomTitle
	}
	return s.Title
}

func (s Session) Key() string {
	return s.ProjectPath + "::" + s.UUID
}
