package ingest

import "time"

// Job is the single normalized record every connector returns.
type Job struct {
	Title           string
	Company         string
	Description     string
	Location        string
	URL             string
	PostedAt        *time.Time
	Source          string
	IsRemote        bool
	RemoteScope     string
	SalaryMin       *int64
	SalaryMax       *int64
	SalaryCurrency  string
	Seniority       string
	RoleFamily      string
	NativeSeniority string
}
