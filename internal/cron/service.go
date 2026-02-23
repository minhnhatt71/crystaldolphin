// Package cron manages scheduled agent tasks.
//
// JSON persistence is byte-compatible with nanobot's Python jobs.json:
//
//	{ "version": 1, "jobs": [ { "id":"…", "name":"…", "enabled":true,
//	    "schedule":{"kind":"every","everyMs":…},
//	    "payload":{"kind":"agent_turn","message":"…","deliver":false},
//	    "state":{"nextRunAtMs":…,"lastRunAtMs":…,"lastStatus":"ok"},
//	    "createdAtMs":…, "updatedAtMs":…, "deleteAfterRun":false } ] }
package cron

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	robfigcron "github.com/robfig/cron/v3"

	"github.com/crystaldolphin/crystaldolphin/internal/tools"
)

// --------------------------------------------------------------------------
// Data types (JSON-compatible with Python)
// --------------------------------------------------------------------------

type CronSchedule struct {
	Kind    string  `json:"kind"`              // "every" | "cron" | "at"
	AtMs    *int64  `json:"atMs,omitempty"`    // one-time
	EveryMs *int64  `json:"everyMs,omitempty"` // interval
	Expr    *string `json:"expr,omitempty"`    // cron expression
	TZ      *string `json:"tz,omitempty"`      // IANA timezone
}

type CronPayload struct {
	Kind    string  `json:"kind"` // "agent_turn"
	Message string  `json:"message"`
	Deliver bool    `json:"deliver"`
	Channel *string `json:"channel,omitempty"`
	To      *string `json:"to,omitempty"`
}

type CronJobState struct {
	NextRunAtMs *int64  `json:"nextRunAtMs,omitempty"`
	LastRunAtMs *int64  `json:"lastRunAtMs,omitempty"`
	LastStatus  *string `json:"lastStatus,omitempty"`
	LastError   *string `json:"lastError,omitempty"`
}

type CronJob struct {
	ID             string       `json:"id"`
	Name           string       `json:"name"`
	Enabled        bool         `json:"enabled"`
	Schedule       CronSchedule `json:"schedule"`
	Payload        CronPayload  `json:"payload"`
	State          CronJobState `json:"state"`
	CreatedAtMs    int64        `json:"createdAtMs"`
	UpdatedAtMs    int64        `json:"updatedAtMs"`
	DeleteAfterRun bool         `json:"deleteAfterRun"`
}

type cronStore struct {
	Version int       `json:"version"`
	Jobs    []CronJob `json:"jobs"`
}

// --------------------------------------------------------------------------
// Service
// --------------------------------------------------------------------------

// OnJobFunc is called when a job fires.  It returns the agent's response text.
type OnJobFunc func(ctx context.Context, job CronJob) (string, error)

// Service manages scheduled jobs.
// It also implements tools.CronServicer so it can be passed to CronTool.
type Service struct {
	storePath string
	onJob     OnJobFunc

	mu    sync.Mutex
	store cronStore

	// Active timers / cron entries keyed by job ID.
	timers    map[string]*time.Timer
	robfig    *robfigcron.Cron
	robfigIDs map[string]robfigcron.EntryID // jobID → robfig entry
}

// NewService creates a CronService.
// storePath is the path to jobs.json (e.g. ~/.nanobot/cron/jobs.json).
func NewService(storePath string) *Service {
	return &Service{
		storePath: storePath,
		timers:    make(map[string]*time.Timer),
		robfig:    robfigcron.New(robfigcron.WithSeconds()),
		robfigIDs: make(map[string]robfigcron.EntryID),
	}
}

// SetOnJob registers the callback executed when a job fires.
// Must be set before Start().
func (s *Service) SetOnJob(fn OnJobFunc) { s.onJob = fn }

// Start loads jobs from disk, (re)computes next-run times, and arms all timers.
// Blocks until ctx is cancelled.
func (s *Service) Start(ctx context.Context) error {
	s.mu.Lock()
	if err := s.loadLocked(); err != nil {
		slog.Warn("cron: load failed, starting empty", "err", err)
	}
	s.recomputeNextRunsLocked()
	s.saveLocked()
	s.armAllLocked(ctx)
	s.mu.Unlock()

	s.robfig.Start()
	slog.Info("cron: started", "jobs", len(s.store.Jobs))

	<-ctx.Done()

	<-s.robfig.Stop().Done()
	s.mu.Lock()
	for _, t := range s.timers {
		t.Stop()
	}
	s.mu.Unlock()
	return ctx.Err()
}

// AddJob adds a new job, saves it, and arms its timer.
// Implements tools.CronServicer.AddJob.
func (s *Service) AddJob(
	name, message, kind string,
	everyMs int64, cronExpr, tz string, atMs int64,
	deliver bool, channel, to string, deleteAfterRun bool,
) (string, error) {
	sched := CronSchedule{Kind: kind}
	switch kind {
	case "every":
		sched.EveryMs = &everyMs
	case "cron":
		sched.Expr = &cronExpr
		if tz != "" {
			sched.TZ = &tz
		}
	case "at":
		sched.AtMs = &atMs
	default:
		return "", fmt.Errorf("unknown schedule kind %q", kind)
	}

	payload := CronPayload{
		Kind:    "agent_turn",
		Message: message,
		Deliver: deliver,
	}
	if channel != "" {
		payload.Channel = &channel
	}
	if to != "" {
		payload.To = &to
	}

	now := nowMs()
	id := shortID()
	nextRun := computeNextRun(sched, now)
	job := CronJob{
		ID:             id,
		Name:           name,
		Enabled:        true,
		Schedule:       sched,
		Payload:        payload,
		State:          CronJobState{NextRunAtMs: nextRun},
		CreatedAtMs:    now,
		UpdatedAtMs:    now,
		DeleteAfterRun: deleteAfterRun,
	}

	s.mu.Lock()
	s.store.Jobs = append(s.store.Jobs, job)
	s.saveLocked()
	s.mu.Unlock()

	slog.Info("cron: added job", "name", name, "id", id, "kind", kind)
	return id, nil
}

// ListJobs returns summaries of all enabled jobs.
// Implements tools.CronServicer.ListJobs.
func (s *Service) ListJobs() []tools.CronJobSummary {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []tools.CronJobSummary
	for _, j := range s.store.Jobs {
		if !j.Enabled {
			continue
		}
		out = append(out, tools.CronJobSummary{ID: j.ID, Name: j.Name, Kind: j.Schedule.Kind})
	}
	return out
}

// RemoveJob removes a job by ID and returns true if found.
// Implements tools.CronServicer.RemoveJob.
func (s *Service) RemoveJob(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	before := len(s.store.Jobs)
	filtered := s.store.Jobs[:0]
	for _, j := range s.store.Jobs {
		if j.ID != id {
			filtered = append(filtered, j)
		}
	}
	s.store.Jobs = filtered
	if len(filtered) < before {
		s.cancelTimerLocked(id)
		s.saveLocked()
		return true
	}
	return false
}

// --------------------------------------------------------------------------
// CLI-facing helpers (used by cmd/cron.go)
// --------------------------------------------------------------------------

// ListAllJobs returns all jobs; includeDisabled controls visibility.
func (s *Service) ListAllJobs(includeDisabled bool) []CronJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.loadLocked() // ensure loaded
	var jobs []CronJob
	for _, j := range s.store.Jobs {
		if includeDisabled || j.Enabled {
			jobs = append(jobs, j)
		}
	}
	sort.Slice(jobs, func(i, k int) bool {
		a := int64(^uint64(0) >> 1)
		b := int64(^uint64(0) >> 1)
		if jobs[i].State.NextRunAtMs != nil {
			a = *jobs[i].State.NextRunAtMs
		}
		if jobs[k].State.NextRunAtMs != nil {
			b = *jobs[k].State.NextRunAtMs
		}
		return a < b
	})
	return jobs
}

// AddJobFull is the CLI-level add (takes a fully-formed CronJob minus ID/times).
func (s *Service) AddJobFull(name, message, kind string, everyMs int64, cronExpr, tz string, atMs int64,
	deliver bool, channel, to string, deleteAfterRun bool) (CronJob, error) {
	id, err := s.AddJob(name, message, kind, everyMs, cronExpr, tz, atMs, deliver, channel, to, deleteAfterRun)
	if err != nil {
		return CronJob{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, j := range s.store.Jobs {
		if j.ID == id {
			return j, nil
		}
	}
	return CronJob{}, fmt.Errorf("job not found after add")
}

// EnableJob enables or disables a job.
func (s *Service) EnableJob(id string, enabled bool) (CronJob, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.store.Jobs {
		if s.store.Jobs[i].ID == id {
			s.store.Jobs[i].Enabled = enabled
			s.store.Jobs[i].UpdatedAtMs = nowMs()
			if enabled {
				next := computeNextRun(s.store.Jobs[i].Schedule, nowMs())
				s.store.Jobs[i].State.NextRunAtMs = next
			} else {
				s.store.Jobs[i].State.NextRunAtMs = nil
				s.cancelTimerLocked(id)
			}
			s.saveLocked()
			return s.store.Jobs[i], true
		}
	}
	return CronJob{}, false
}

// RunJob manually executes a job (force=true ignores disabled flag).
func (s *Service) RunJob(ctx context.Context, id string, force bool) bool {
	s.mu.Lock()
	var job *CronJob
	for i := range s.store.Jobs {
		if s.store.Jobs[i].ID == id {
			if !force && !s.store.Jobs[i].Enabled {
				s.mu.Unlock()
				return false
			}
			job = &s.store.Jobs[i]
			break
		}
	}
	if job == nil {
		s.mu.Unlock()
		return false
	}
	jobCopy := *job
	s.mu.Unlock()

	s.executeJob(ctx, jobCopy)
	return true
}

// --------------------------------------------------------------------------
// Internal scheduling logic
// --------------------------------------------------------------------------

func (s *Service) recomputeNextRunsLocked() {
	now := nowMs()
	for i := range s.store.Jobs {
		if s.store.Jobs[i].Enabled {
			s.store.Jobs[i].State.NextRunAtMs = computeNextRun(s.store.Jobs[i].Schedule, now)
		}
	}
}

func (s *Service) armAllLocked(ctx context.Context) {
	for _, j := range s.store.Jobs {
		if j.Enabled {
			s.armJobLocked(ctx, j)
		}
	}
}

func (s *Service) armJobLocked(ctx context.Context, job CronJob) {
	s.cancelTimerLocked(job.ID)

	switch job.Schedule.Kind {
	case "every":
		if job.Schedule.EveryMs == nil || *job.Schedule.EveryMs <= 0 {
			return
		}
		d := time.Duration(*job.Schedule.EveryMs) * time.Millisecond
		t := time.AfterFunc(d, func() {
			s.executeJob(ctx, job)
			// Re-arm for next tick.
			s.mu.Lock()
			// Refresh job from store in case it changed.
			for _, j := range s.store.Jobs {
				if j.ID == job.ID && j.Enabled {
					s.armJobLocked(ctx, j)
					break
				}
			}
			s.mu.Unlock()
		})
		s.timers[job.ID] = t

	case "at":
		if job.Schedule.AtMs == nil {
			return
		}
		delay := time.Until(time.UnixMilli(*job.Schedule.AtMs))
		if delay < 0 {
			return
		}
		t := time.AfterFunc(delay, func() {
			s.executeJob(ctx, job)
		})
		s.timers[job.ID] = t

	case "cron":
		if job.Schedule.Expr == nil {
			return
		}
		loc := time.Local
		if job.Schedule.TZ != nil && *job.Schedule.TZ != "" {
			if l, err := time.LoadLocation(*job.Schedule.TZ); err == nil {
				loc = l
			}
		}
		expr := robfigcron.NewParser(
			robfigcron.Minute | robfigcron.Hour | robfigcron.Dom | robfigcron.Month | robfigcron.Dow,
		)
		sched, err := expr.Parse(*job.Schedule.Expr)
		if err != nil {
			slog.Warn("cron: invalid cron expression", "job", job.ID, "expr", *job.Schedule.Expr, "err", err)
			return
		}
		jobCopy := job
		entryID := s.robfig.Schedule(
			withLocation(sched, loc),
			robfigcron.FuncJob(func() { s.executeJob(ctx, jobCopy) }),
		)
		s.robfigIDs[job.ID] = entryID
	}
}

func (s *Service) cancelTimerLocked(id string) {
	if t, ok := s.timers[id]; ok {
		t.Stop()
		delete(s.timers, id)
	}
	if eid, ok := s.robfigIDs[id]; ok {
		s.robfig.Remove(eid)
		delete(s.robfigIDs, id)
	}
}

func (s *Service) executeJob(ctx context.Context, job CronJob) {
	startMs := nowMs()
	slog.Info("cron: executing job", "name", job.Name, "id", job.ID)

	var lastStatus = "ok"
	var lastErr *string

	if s.onJob != nil {
		if _, err := s.onJob(ctx, job); err != nil {
			lastStatus = "error"
			e := err.Error()
			lastErr = &e
			slog.Error("cron: job failed", "name", job.Name, "err", err)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.store.Jobs {
		if s.store.Jobs[i].ID != job.ID {
			continue
		}
		now := nowMs()
		s.store.Jobs[i].State.LastRunAtMs = &startMs
		s.store.Jobs[i].State.LastStatus = &lastStatus
		s.store.Jobs[i].State.LastError = lastErr
		s.store.Jobs[i].UpdatedAtMs = now

		if job.Schedule.Kind == "at" {
			if job.DeleteAfterRun {
				// Remove from slice.
				filtered := s.store.Jobs[:0]
				for _, j := range s.store.Jobs {
					if j.ID != job.ID {
						filtered = append(filtered, j)
					}
				}
				s.store.Jobs = filtered
			} else {
				s.store.Jobs[i].Enabled = false
				s.store.Jobs[i].State.NextRunAtMs = nil
			}
		} else {
			next := computeNextRun(job.Schedule, now)
			s.store.Jobs[i].State.NextRunAtMs = next
		}
		break
	}
	s.saveLocked()
}

// --------------------------------------------------------------------------
// Persistence
// --------------------------------------------------------------------------

func (s *Service) loadLocked() error {
	if len(s.store.Jobs) > 0 {
		return nil // already loaded
	}
	data, err := os.ReadFile(s.storePath)
	if os.IsNotExist(err) {
		s.store = cronStore{Version: 1}
		return nil
	}
	if err != nil {
		return err
	}
	var st cronStore
	if err := json.Unmarshal(data, &st); err != nil {
		return err
	}
	if st.Version == 0 {
		st.Version = 1
	}
	s.store = st
	return nil
}

func (s *Service) saveLocked() {
	if err := os.MkdirAll(filepath.Dir(s.storePath), 0o755); err != nil {
		slog.Warn("cron: mkdir failed", "err", err)
		return
	}
	data, err := json.MarshalIndent(s.store, "", "  ")
	if err != nil {
		slog.Warn("cron: marshal failed", "err", err)
		return
	}
	if err := os.WriteFile(s.storePath, data, 0o644); err != nil {
		slog.Warn("cron: write failed", "err", err)
	}
}

// --------------------------------------------------------------------------
// Utility
// --------------------------------------------------------------------------

func nowMs() int64 { return time.Now().UnixMilli() }

func shortID() string {
	return fmt.Sprintf("%08x", time.Now().UnixNano()&0xFFFFFFFF)
}

// computeNextRun mirrors Python's _compute_next_run.
func computeNextRun(sched CronSchedule, nowMs int64) *int64 {
	switch sched.Kind {
	case "at":
		if sched.AtMs != nil && *sched.AtMs > nowMs {
			v := *sched.AtMs
			return &v
		}
	case "every":
		if sched.EveryMs != nil && *sched.EveryMs > 0 {
			v := nowMs + *sched.EveryMs
			return &v
		}
	case "cron":
		if sched.Expr != nil {
			loc := time.Local
			if sched.TZ != nil && *sched.TZ != "" {
				if l, err := time.LoadLocation(*sched.TZ); err == nil {
					loc = l
				}
			}
			parser := robfigcron.NewParser(
				robfigcron.Minute | robfigcron.Hour | robfigcron.Dom | robfigcron.Month | robfigcron.Dow,
			)
			parsed, err := parser.Parse(*sched.Expr)
			if err == nil {
				next := parsed.Next(time.UnixMilli(nowMs).In(loc))
				v := next.UnixMilli()
				return &v
			}
		}
	}
	return nil
}

// withLocation wraps a Schedule to always use a specific location.
type locSchedule struct {
	inner robfigcron.Schedule
	loc   *time.Location
}

func (l locSchedule) Next(t time.Time) time.Time {
	return l.inner.Next(t.In(l.loc))
}

func withLocation(s robfigcron.Schedule, loc *time.Location) robfigcron.Schedule {
	return locSchedule{inner: s, loc: loc}
}
