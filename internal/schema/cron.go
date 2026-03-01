package schema

import "github.com/crystaldolphin/crystaldolphin/internal/bus"

// CronJobSummary is a lightweight view of a scheduled job used by the cron tool.
type CronJobSummary struct {
	ID   string
	Name string
	Kind string // "every", "cron", or "at"
}

// CronService is the interface the cron tool uses to manage scheduled jobs.
// Implemented by cron.JobManager. Defined here to avoid an import cycle.
type CronService interface {
	AddJob(
		name, message, kind string,
		everyMs int64, cronExpr, tz string, atMs int64,
		deliver bool, channel bus.Channel, to string, deleteAfterRun bool,
	) (id string, err error)
	ListJobs() []CronJobSummary
	RemoveJob(id string) bool
}
