package cron

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// newTestManager creates a JobManager backed by a temp file.
func newTestManager(t *testing.T) (*JobManager, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "jobs.json")
	return NewService(path), path
}

// startManager starts the manager in the background and returns a cancel func.
func startManager(t *testing.T, m *JobManager) context.CancelFunc {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = m.Start(ctx) }()
	// Give Start() a moment to arm timers.
	time.Sleep(20 * time.Millisecond)
	return cancel
}

// ─── AddJob ────────────────────────────────────────────────────────────────

func TestAddJob_Every(t *testing.T) {
	m, _ := newTestManager(t)
	id, err := m.AddJob("tick", "hello", "every", 5000, "", "", 0, false, "", "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty id")
	}
	jobs := m.ListAllJobs(false)
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].Schedule.Kind != "every" {
		t.Errorf("expected kind=every, got %q", jobs[0].Schedule.Kind)
	}
	if jobs[0].Schedule.EveryMs == nil || *jobs[0].Schedule.EveryMs != 5000 {
		t.Errorf("unexpected everyMs: %v", jobs[0].Schedule.EveryMs)
	}
}

func TestAddJob_At(t *testing.T) {
	m, _ := newTestManager(t)
	futureMs := time.Now().Add(time.Hour).UnixMilli()
	id, err := m.AddJob("once", "do it", "at", 0, "", "", futureMs, false, "", "", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	jobs := m.ListAllJobs(false)
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].ID != id {
		t.Errorf("id mismatch: got %q", jobs[0].ID)
	}
	if !jobs[0].DeleteAfterRun {
		t.Error("expected deleteAfterRun=true")
	}
}

func TestAddJob_Cron(t *testing.T) {
	m, _ := newTestManager(t)
	id, err := m.AddJob("daily", "report", "cron", 0, "0 9 * * *", "UTC", 0, true, "telegram", "123", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	jobs := m.ListAllJobs(false)
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].ID != id {
		t.Errorf("id mismatch")
	}
	if jobs[0].Payload.Deliver != true {
		t.Error("expected deliver=true")
	}
	if jobs[0].Payload.Channel == nil || *jobs[0].Payload.Channel != "telegram" {
		t.Errorf("unexpected channel: %v", jobs[0].Payload.Channel)
	}
}

func TestAddJob_UnknownKind(t *testing.T) {
	m, _ := newTestManager(t)
	_, err := m.AddJob("bad", "msg", "weekly", 0, "", "", 0, false, "", "", false)
	if err == nil {
		t.Fatal("expected error for unknown kind")
	}
}

// ─── RemoveJob ─────────────────────────────────────────────────────────────

func TestRemoveJob_Exists(t *testing.T) {
	m, _ := newTestManager(t)
	id, _ := m.AddJob("job", "msg", "every", 1000, "", "", 0, false, "", "", false)
	if !m.RemoveJob(id) {
		t.Fatal("expected RemoveJob to return true")
	}
	if len(m.ListAllJobs(false)) != 0 {
		t.Error("expected empty job list after remove")
	}
}

func TestRemoveJob_NotFound(t *testing.T) {
	m, _ := newTestManager(t)
	if m.RemoveJob("nonexistent") {
		t.Fatal("expected RemoveJob to return false for unknown id")
	}
}

// ─── ListJobs ──────────────────────────────────────────────────────────────

func TestListJobs_OnlyEnabled(t *testing.T) {
	m, _ := newTestManager(t)
	m.AddJob("a", "msg", "every", 1000, "", "", 0, false, "", "", false)
	id2, _ := m.AddJob("b", "msg", "every", 2000, "", "", 0, false, "", "", false)
	m.EnableJob(id2, false)

	summaries := m.ListJobs()
	if len(summaries) != 1 {
		t.Fatalf("expected 1 enabled job summary, got %d", len(summaries))
	}
	if summaries[0].Name != "a" {
		t.Errorf("unexpected job name: %q", summaries[0].Name)
	}
}

// ─── EnableJob ─────────────────────────────────────────────────────────────

func TestEnableJob_ToggleDisableEnable(t *testing.T) {
	m, _ := newTestManager(t)
	id, _ := m.AddJob("j", "msg", "every", 1000, "", "", 0, false, "", "", false)

	job, ok := m.EnableJob(id, false)
	if !ok {
		t.Fatal("EnableJob returned false")
	}
	if job.Enabled {
		t.Error("expected job to be disabled")
	}
	if job.State.NextRunAtMs != nil {
		t.Error("expected nil NextRunAtMs when disabled")
	}

	job, ok = m.EnableJob(id, true)
	if !ok {
		t.Fatal("EnableJob returned false on re-enable")
	}
	if !job.Enabled {
		t.Error("expected job to be enabled")
	}
}

func TestEnableJob_NotFound(t *testing.T) {
	m, _ := newTestManager(t)
	_, ok := m.EnableJob("ghost", true)
	if ok {
		t.Fatal("expected ok=false for unknown id")
	}
}

// ─── ListAllJobs ───────────────────────────────────────────────────────────

func TestListAllJobs_IncludeDisabled(t *testing.T) {
	m, _ := newTestManager(t)
	id, _ := m.AddJob("j", "msg", "every", 1000, "", "", 0, false, "", "", false)
	m.EnableJob(id, false)

	all := m.ListAllJobs(true)
	if len(all) != 1 {
		t.Fatalf("expected 1 job with includeDisabled=true, got %d", len(all))
	}
	filtered := m.ListAllJobs(false)
	if len(filtered) != 0 {
		t.Fatalf("expected 0 jobs with includeDisabled=false, got %d", len(filtered))
	}
}

func TestListAllJobs_SortedByNextRun(t *testing.T) {
	m, _ := newTestManager(t)
	// Add two "every" jobs; the second fires sooner.
	m.AddJob("slow", "msg", "every", 60000, "", "", 0, false, "", "", false)
	m.AddJob("fast", "msg", "every", 1000, "", "", 0, false, "", "", false)

	jobs := m.ListAllJobs(false)
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}
	if *jobs[0].State.NextRunAtMs > *jobs[1].State.NextRunAtMs {
		t.Error("jobs not sorted by NextRunAtMs ascending")
	}
}

// ─── Persistence ───────────────────────────────────────────────────────────

func TestPersistence_RoundTrip(t *testing.T) {
	m, path := newTestManager(t)
	id, _ := m.AddJob("persist", "hello", "every", 5000, "", "", 0, false, "", "", false)

	// Read back from disk directly.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read jobs.json: %v", err)
	}
	var store cronStore
	if err := json.Unmarshal(data, &store); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Version is set to 1 only after loadLocked() runs (i.e. on Start or ListAllJobs).
	// AddJob alone doesn't trigger a load, so version may be 0.
	if store.Version != 0 && store.Version != 1 {
		t.Errorf("unexpected version: %d", store.Version)
	}
	if len(store.Jobs) != 1 {
		t.Fatalf("expected 1 persisted job, got %d", len(store.Jobs))
	}
	if store.Jobs[0].ID != id {
		t.Errorf("id mismatch in persisted file")
	}
}

func TestPersistence_LoadExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "jobs.json")
	existing := `{"version":1,"jobs":[{"id":"aabbccdd","name":"loaded","enabled":true,
		"schedule":{"kind":"every","everyMs":3000},"payload":{"kind":"agent_turn","message":"hi","deliver":false},
		"state":{},"createdAtMs":1000,"updatedAtMs":1000,"deleteAfterRun":false}]}`
	os.WriteFile(path, []byte(existing), 0o644)

	m := NewService(path)
	jobs := m.ListAllJobs(false)
	if len(jobs) != 1 {
		t.Fatalf("expected 1 loaded job, got %d", len(jobs))
	}
	if jobs[0].Name != "loaded" {
		t.Errorf("unexpected job name: %q", jobs[0].Name)
	}
}

func TestPersistence_MissingFile(t *testing.T) {
	m, _ := newTestManager(t)
	// No file created; should start empty.
	jobs := m.ListAllJobs(false)
	if len(jobs) != 0 {
		t.Fatalf("expected 0 jobs from missing file, got %d", len(jobs))
	}
}

// ─── computeNextRun ────────────────────────────────────────────────────────

func TestComputeNextRun_Every(t *testing.T) {
	everyMs := int64(5000)
	now := int64(1_000_000)
	sched := CronSchedule{Kind: "every", EveryMs: &everyMs}
	result := computeNextRun(sched, now)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if *result != now+everyMs {
		t.Errorf("expected %d, got %d", now+everyMs, *result)
	}
}

func TestComputeNextRun_At_Future(t *testing.T) {
	future := time.Now().Add(time.Hour).UnixMilli()
	sched := CronSchedule{Kind: "at", AtMs: &future}
	result := computeNextRun(sched, time.Now().UnixMilli())
	if result == nil || *result != future {
		t.Errorf("expected future=%d, got %v", future, result)
	}
}

func TestComputeNextRun_At_Past(t *testing.T) {
	past := time.Now().Add(-time.Hour).UnixMilli()
	sched := CronSchedule{Kind: "at", AtMs: &past}
	result := computeNextRun(sched, time.Now().UnixMilli())
	if result != nil {
		t.Errorf("expected nil for past at-job, got %d", *result)
	}
}

func TestComputeNextRun_Cron_UTC(t *testing.T) {
	expr := "0 12 * * *"
	tz := "UTC"
	sched := CronSchedule{Kind: "cron", Expr: &expr, TZ: &tz}
	result := computeNextRun(sched, time.Now().UnixMilli())
	if result == nil {
		t.Fatal("expected non-nil cron next run")
	}
	if *result <= time.Now().UnixMilli() {
		t.Error("next run should be in the future")
	}
}

func TestComputeNextRun_Cron_InvalidExpr(t *testing.T) {
	expr := "not a cron"
	sched := CronSchedule{Kind: "cron", Expr: &expr}
	result := computeNextRun(sched, time.Now().UnixMilli())
	if result != nil {
		t.Error("expected nil for invalid cron expression")
	}
}

func TestComputeNextRun_Every_ZeroInterval(t *testing.T) {
	everyMs := int64(0)
	sched := CronSchedule{Kind: "every", EveryMs: &everyMs}
	result := computeNextRun(sched, time.Now().UnixMilli())
	if result != nil {
		t.Error("expected nil for zero interval")
	}
}

// ─── Job execution ─────────────────────────────────────────────────────────

func TestExecuteJob_CallsOnJob(t *testing.T) {
	m, _ := newTestManager(t)

	var called atomic.Int32
	m.OnJobFunc(func(_ context.Context, job CronJob) (string, error) {
		called.Add(1)
		return "ok", nil
	})

	id, _ := m.AddJob("run", "msg", "every", 10000, "", "", 0, false, "", "", false)
	cancel := startManager(t, m)
	defer cancel()

	ctx := context.Background()
	if !m.RunJob(ctx, id, true) {
		t.Fatal("RunJob returned false")
	}

	// Wait for callback.
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) && called.Load() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if called.Load() == 0 {
		t.Error("onJob was not called")
	}
}

func TestExecuteJob_UpdatesState(t *testing.T) {
	m, _ := newTestManager(t)
	m.OnJobFunc(func(_ context.Context, _ CronJob) (string, error) { return "done", nil })

	id, _ := m.AddJob("state", "msg", "every", 10000, "", "", 0, false, "", "", false)
	cancel := startManager(t, m)
	defer cancel()

	m.RunJob(context.Background(), id, true)

	// Give executeJob goroutine time to finish.
	time.Sleep(50 * time.Millisecond)

	jobs := m.ListAllJobs(false)
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].State.LastRunAtMs == nil {
		t.Error("expected LastRunAtMs to be set after execution")
	}
	if jobs[0].State.LastStatus == nil || *jobs[0].State.LastStatus != "ok" {
		t.Errorf("unexpected status: %v", jobs[0].State.LastStatus)
	}
}

func TestExecuteJob_AtDeleteAfterRun(t *testing.T) {
	m, _ := newTestManager(t)
	m.OnJobFunc(func(_ context.Context, _ CronJob) (string, error) { return "", nil })

	futureMs := time.Now().Add(time.Hour).UnixMilli()
	id, _ := m.AddJob("once", "msg", "at", 0, "", "", futureMs, false, "", "", true)
	cancel := startManager(t, m)
	defer cancel()

	m.RunJob(context.Background(), id, true)
	time.Sleep(50 * time.Millisecond)

	jobs := m.ListAllJobs(true)
	if len(jobs) != 0 {
		t.Errorf("expected job deleted after run, got %d jobs", len(jobs))
	}
}

func TestRunJob_DisabledWithoutForce(t *testing.T) {
	m, _ := newTestManager(t)
	id, _ := m.AddJob("j", "msg", "every", 10000, "", "", 0, false, "", "", false)
	m.EnableJob(id, false)
	cancel := startManager(t, m)
	defer cancel()

	if m.RunJob(context.Background(), id, false) {
		t.Error("expected RunJob to return false for disabled job without force")
	}
}

func TestRunJob_NotFound(t *testing.T) {
	m, _ := newTestManager(t)
	cancel := startManager(t, m)
	defer cancel()

	if m.RunJob(context.Background(), "ghost", false) {
		t.Error("expected RunJob to return false for unknown id")
	}
}

// ─── Timer firing ──────────────────────────────────────────────────────────

func TestEveryJob_FiresAfterInterval(t *testing.T) {
	m, _ := newTestManager(t)

	var count atomic.Int32
	m.OnJobFunc(func(_ context.Context, _ CronJob) (string, error) {
		count.Add(1)
		return "", nil
	})

	m.AddJob("fast", "msg", "every", 50, "", "", 0, false, "", "", false)
	cancel := startManager(t, m)
	defer cancel()

	time.Sleep(180 * time.Millisecond)
	if n := count.Load(); n < 2 {
		t.Errorf("expected at least 2 executions, got %d", n)
	}
}

func TestAtJob_FiresOnce(t *testing.T) {
	m, _ := newTestManager(t)

	var count atomic.Int32
	m.OnJobFunc(func(_ context.Context, _ CronJob) (string, error) {
		count.Add(1)
		return "", nil
	})

	atMs := time.Now().Add(50 * time.Millisecond).UnixMilli()
	m.AddJob("once", "msg", "at", 0, "", "", atMs, false, "", "", false)
	cancel := startManager(t, m)
	defer cancel()

	time.Sleep(200 * time.Millisecond)
	if n := count.Load(); n != 1 {
		t.Errorf("expected exactly 1 execution for at-job, got %d", n)
	}
}

// ─── AddJobFull ────────────────────────────────────────────────────────────

func TestAddJobFull_ReturnsJob(t *testing.T) {
	m, _ := newTestManager(t)
	job, err := m.AddJobFull("full", "msg", "every", 1000, "", "", 0, false, "", "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if job.Name != "full" {
		t.Errorf("unexpected name: %q", job.Name)
	}
	if job.ID == "" {
		t.Error("expected non-empty id")
	}
}
