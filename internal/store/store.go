package store

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"sync"

	"github.com/your/module/internal/model"
)

type perServerLogs struct {
	Info  []model.LogLine
	Debug []model.LogLine
	Error []model.LogLine
}

// Store is a minimal in-memory view for demo/testing.
type Store struct {
	mu      sync.RWMutex
	servers map[string]*model.Server
	logs    map[string]*perServerLogs
	subs    map[string]map[chan model.LogLine]struct{} // key: serverID|level
}

func New() *Store {
	return &Store{
		servers: map[string]*model.Server{},
		logs:    map[string]*perServerLogs{},
		subs:    map[string]map[chan model.LogLine]struct{}{},
	}
}

func key(id, level string) string { return id + "|" + level }

func (s *Store) UpsertServer(srv *model.Server) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := model.NowISO()
	if srv.CreatedAt == "" {
		srv.CreatedAt = now
	}
	srv.UpdatedAt = now
	if srv.Status == "" {
		srv.Status = "running"
	}
	s.servers[srv.ID] = srv
	if _, ok := s.logs[srv.ID]; !ok {
		s.logs[srv.ID] = &perServerLogs{}
	}
}

func (s *Store) GetServers() []*model.Server {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*model.Server, 0, len(s.servers))
	for _, v := range s.servers {
		out = append(out, v)
	}
	return out
}

func (s *Store) GetServer(id string) (*model.Server, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.servers[id]
	return v, ok
}

func (s *Store) AppendLog(id, level, msg string) (model.LogLine, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.logs[id]
	if !ok {
		return model.LogLine{}, errors.New("unknown server")
	}
	line := model.LogLine{T: model.NowISO(), LVL: level, MSG: msg}
	switch level {
	case "info":
		l.Info = append(l.Info, line)
	case "debug":
		l.Debug = append(l.Debug, line)
	case "error":
		l.Error = append(l.Error, line)
	default:
		l.Info = append(l.Info, line)
	}
	// notify subscribers
	k := key(id, level)
	for ch := range s.subs[k] {
		select {
		case ch <- line:
		default:
		}
	}
	return line, nil
}

func (s *Store) GetLogs(id, level string, limit int) ([]model.LogLine, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	l, ok := s.logs[id]
	if !ok {
		return nil, errors.New("unknown server")
	}
	var src []model.LogLine
	switch level {
	case "debug":
		src = l.Debug
	case "error":
		src = l.Error
	default:
		src = l.Info
	}
	if limit <= 0 || limit >= len(src) {
		return append([]model.LogLine(nil), src...), nil
	}
	return append([]model.LogLine(nil), src[len(src)-limit:]...), nil
}

func (s *Store) SubscribeLogs(ctx context.Context, id, level string) (<-chan model.LogLine, func()) {
	ch := make(chan model.LogLine, 64)
	var once sync.Once
	closeCh := func() { once.Do(func() { close(ch) }) }

	k := key(id, level)
	s.mu.Lock()
	if _, ok := s.subs[k]; !ok {
		s.subs[k] = map[chan model.LogLine]struct{}{}
	}
	s.subs[k][ch] = struct{}{}
	s.mu.Unlock()

	cancel := func() {
		s.mu.Lock()
		delete(s.subs[k], ch)
		s.mu.Unlock()
		closeCh()
	}

	go func() {
		<-ctx.Done()
		cancel()
	}()
	return ch, cancel
}

// Seed minimal demo state.
func (s *Store) SeedDemo() {
	srv := &model.Server{
		ID:        "demo-1",
		Name:      "GuildNet Agent",
		Image:     "guildnet/agent:dev",
		Status:    "running",
		Ports:     []model.Port{{Name: "http", Port: 8080}, {Name: "https", Port: 8443}},
		Resources: &model.Resources{CPU: "500m", Memory: "256Mi"},
		Env:       map[string]string{"ENV": "dev", "AGENT_HOST": "127.0.0.1"},
	}
	s.UpsertServer(srv)
	// add some logs
	for i := 0; i < 20; i++ {
		lvl := []string{"info", "debug", "error"}[rand.Intn(3)]
		msg := struct {
			I int
			L string
		}{I: i, L: lvl}
		b, _ := json.Marshal(msg)
		_, _ = s.AppendLog(srv.ID, lvl, string(b))
	}
}
