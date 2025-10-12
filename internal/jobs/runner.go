package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Status enumerates job states.
type Status string

const (
	Queued    Status = "queued"
	Running   Status = "running"
	Succeeded Status = "succeeded"
	Failed    Status = "failed"
	Canceled  Status = "canceled"
)

// LogEvent represents a structured log message emitted by a job step.
type LogEvent struct {
	TS   time.Time      `json:"ts"`
	Job  string         `json:"job"`
	Step string         `json:"step,omitempty"`
	Msg  string         `json:"msg"`
	KV   map[string]any `json:"kv,omitempty"`
	Err  string         `json:"err,omitempty"`
}

// Persist abstracts durable storage needed by Runner.
type Persist interface {
	SaveJob(rec Record) error
	AppendLog(jobID string, e LogEvent) error
	ListJobs() ([]Record, error)
	GetJob(id string) (*Record, error)
}

// Runner is an in-memory job orchestrator with resumable checkpoint support.
type Runner struct {
	mu       sync.RWMutex
	jobs     map[string]*Record
	queues   map[string]chan string // per-kind queue
	logSubs  map[string][]chan LogEvent
	store    Persist
	canceled map[string]struct{}
}

type Record struct {
	ID       string          `json:"id"`
	Kind     string          `json:"kind"`
	SpecJSON string          `json:"specJSON"`
	Status   Status          `json:"status"`
	Progress float64         `json:"progress"`
	Created  time.Time       `json:"created"`
	Updated  time.Time       `json:"updated"`
	Result   json.RawMessage `json:"result,omitempty"`
	Error    string          `json:"error,omitempty"`
}

func New(opts ...Option) *Runner {
	r := &Runner{
		jobs:     map[string]*Record{},
		queues:   map[string]chan string{},
		logSubs:  map[string][]chan LogEvent{},
		canceled: map[string]struct{}{},
	}
	for _, o := range opts {
		o(r)
	}
	return r
}

type Option func(*Runner)

func WithPersist(p Persist) Option { return func(r *Runner) { r.store = p } }

// Submit enqueues a job of a given kind with spec.
func (r *Runner) Submit(kind string, spec any, handler func(ctx context.Context, rec *Record, logf func(step, msg string, kv map[string]any))) (string, error) {
	b, _ := json.Marshal(spec)
	id := uuid.NewString()
	rec := &Record{ID: id, Kind: kind, SpecJSON: string(b), Status: Queued, Created: time.Now(), Updated: time.Now()}
	r.mu.Lock()
	r.jobs[id] = rec
	q := r.ensureQueue(kind)
	r.mu.Unlock()
	r.persist(*rec)
	// Start a worker per kind if not started
	go r.worker(kind, handler)
	// enqueue
	q <- id
	return id, nil
}

func (r *Runner) ensureQueue(kind string) chan string {
	if q, ok := r.queues[kind]; ok {
		return q
	}
	q := make(chan string, 64)
	r.queues[kind] = q
	return q
}

func (r *Runner) worker(kind string, handler func(ctx context.Context, rec *Record, logf func(step, msg string, kv map[string]any))) {
	q := r.ensureQueue(kind)
	for id := range q {
		rec := r.Get(id)
		if rec == nil {
			continue
		}
		r.runOne(handler, rec)
	}
}

func (r *Runner) runOne(handler func(ctx context.Context, rec *Record, logf func(step, msg string, kv map[string]any)), rec *Record) {
	rec.Status = Running
	rec.Updated = time.Now()
	r.put(rec)
	r.persist(*rec)
	ctx := context.Background()
	logf := func(step, msg string, kv map[string]any) {
		e := LogEvent{TS: time.Now(), Job: rec.ID, Step: step, Msg: msg, KV: kv}
		r.publish(rec.ID, e)
		r.append(e)
	}
	defer func() {
		if v := recover(); v != nil {
			rec.Status = Failed
			rec.Error = fmt.Sprint(v)
			rec.Updated = time.Now()
			r.put(rec)
			r.persist(*rec)
		}
	}()
	// run handler
	handler(ctx, rec, logf)
	if rec.Status == Running {
		rec.Status = Succeeded
		rec.Progress = 1
		rec.Updated = time.Now()
		r.put(rec)
		r.persist(*rec)
	}
}

// Get returns a copy of job record by id.
func (r *Runner) Get(id string) *Record {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if rec, ok := r.jobs[id]; ok {
		cpy := *rec
		return &cpy
	}
	if r.store != nil {
		if rec, _ := r.store.GetJob(id); rec != nil {
			return rec
		}
	}
	return nil
}

// List returns all jobs snapshot.
func (r *Runner) List() []Record {
	r.mu.RLock()
	out := make([]Record, 0, len(r.jobs))
	for _, rec := range r.jobs {
		cpy := *rec
		out = append(out, cpy)
	}
	r.mu.RUnlock()
	if r.store != nil {
		if persisted, err := r.store.ListJobs(); err == nil {
			// merge records by ID, prefer in-memory for recency
			seen := map[string]struct{}{}
			for _, m := range out {
				seen[m.ID] = struct{}{}
			}
			for _, p := range persisted {
				if _, has := seen[p.ID]; !has {
					out = append(out, p)
				}
			}
		}
	}
	return out
}

func (r *Runner) put(rec *Record) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cpy := *rec
	r.jobs[rec.ID] = &cpy
}

// SubscribeLogs returns a channel of log events for a job.
func (r *Runner) SubscribeLogs(jobID string) (<-chan LogEvent, func()) {
	ch := make(chan LogEvent, 128)
	r.mu.Lock()
	r.logSubs[jobID] = append(r.logSubs[jobID], ch)
	r.mu.Unlock()
	cancel := func() {
		r.mu.Lock()
		arr := r.logSubs[jobID]
		for i := range arr {
			if arr[i] == ch {
				arr = append(arr[:i], arr[i+1:]...)
				break
			}
		}
		r.logSubs[jobID] = arr
		r.mu.Unlock()
		close(ch)
	}
	return ch, cancel
}

func (r *Runner) publish(jobID string, e LogEvent) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, ch := range r.logSubs[jobID] {
		select {
		case ch <- e:
		default:
		}
	}
}

// WithStep updates job progress and emits a log.
func (r *Runner) WithStep(rec *Record, progress float64, step, msg string, kv map[string]any) {
	rec.Progress = progress
	rec.Updated = time.Now()
	r.put(rec)
	r.persist(*rec)
	e := LogEvent{TS: time.Now(), Job: rec.ID, Step: step, Msg: msg, KV: kv}
	r.publish(rec.ID, e)
	r.append(e)
}

// Fail marks a job as failed with error.
func (r *Runner) Fail(rec *Record, err error) {
	rec.Status = Failed
	rec.Error = err.Error()
	rec.Updated = time.Now()
	r.put(rec)
	r.persist(*rec)
	r.publish(rec.ID, LogEvent{TS: time.Now(), Job: rec.ID, Msg: "failed", Err: err.Error()})
}

func (r *Runner) persist(rec Record) {
	if r.store != nil {
		_ = r.store.SaveJob(rec)
	}
}

func (r *Runner) append(e LogEvent) {
	if r.store != nil {
		_ = r.store.AppendLog(e.Job, e)
	}
}

// Cancel marks a job for cancellation; handlers may observe and stop early.
func (r *Runner) Cancel(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.canceled == nil {
		r.canceled = make(map[string]struct{})
	}
	r.canceled[id] = struct{}{}
	if rec, ok := r.jobs[id]; ok {
		rec.Status = Canceled
		rec.Updated = time.Now()
		r.persist(*rec)
	}
}

// IsCanceled returns true if job was requested to cancel.
func (r *Runner) IsCanceled(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.canceled[id]
	return ok
}
