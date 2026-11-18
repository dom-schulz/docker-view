package ui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dom-schulz/docker-view/internal/docker"
)

const (
	logTail    = 500  // history fetched per container when a project is selected
	logCap     = 5000 // lines kept per project; older lines are dropped
	batchLimit = 2000 // most events delivered to Update in one message
)

// streamEvent is what log and stats goroutines send to the UI.
type streamEvent struct {
	line   *docker.LogLine
	stats  *docker.Stats
	ended  string // container ID whose stream finished
	failed error
}

// streamBatchMsg carries events from one session generation.
type streamBatchMsg struct {
	gen    int
	events []streamEvent
}

// session is a set of goroutines streaming logs or stats for one project.
// Cancelling it stops them all; a new session gets a new generation so stale
// messages are ignored.
type session struct {
	gen     int
	project string
	ctx     context.Context
	cancel  context.CancelFunc
	ch      chan streamEvent
	active  map[string]bool // container ID -> stream running
}

var sessionGen int

func newSession(project string) *session {
	sessionGen++
	ctx, cancel := context.WithCancel(context.Background())
	return &session{
		gen:     sessionGen,
		project: project,
		ctx:     ctx,
		cancel:  cancel,
		ch:      make(chan streamEvent, 1024),
		active:  map[string]bool{},
	}
}

func (s *session) stop() {
	if s != nil {
		s.cancel()
	}
}

func (s *session) send(ev streamEvent) bool {
	select {
	case s.ch <- ev:
		return true
	case <-s.ctx.Done():
		return false
	}
}

// wait returns a command that delivers the next batch of events.
func (s *session) wait() tea.Cmd {
	return func() tea.Msg {
		var first streamEvent
		select {
		case first = <-s.ch:
		case <-s.ctx.Done():
			return nil
		}
		batch := []streamEvent{first}
		for len(batch) < batchLimit {
			select {
			case ev := <-s.ch:
				batch = append(batch, ev)
			default:
				return streamBatchMsg{gen: s.gen, events: batch}
			}
		}
		return streamBatchMsg{gen: s.gen, events: batch}
	}
}

// containerLog remembers where each container's log stream got to, so a
// container that restarts is resumed rather than fetched again.
type containerLog struct {
	fetched   bool
	last      time.Time
	skipUntil time.Time // the daemon's "since" is inclusive, so drop repeats on resume
}

// logSession is the selected project's merged log buffer.
type logSession struct {
	*session
	lines      []docker.LogLine
	containers map[string]*containerLog
	services   []string // service names in first-seen order, for colors
}

func newLogSession(project string) *logSession {
	return &logSession{session: newSession(project), containers: map[string]*containerLog{}}
}

// sync starts streams for containers that need one: every container not yet
// fetched, and running containers whose earlier stream ended.
func (l *logSession) sync(cli *docker.Client, p docker.Project) {
	for _, svc := range p.Services {
		l.colorIndex(svc.Name)
		id := svc.ContainerID
		if l.active[id] {
			continue
		}
		st := l.containers[id]
		if st == nil {
			st = &containerLog{}
			l.containers[id] = st
		}
		if st.fetched && !svc.Running() {
			continue
		}
		opts := docker.LogOptions{Tail: logTail, Follow: true}
		if st.fetched && !st.last.IsZero() {
			opts = docker.LogOptions{Since: st.last, Follow: true}
			st.skipUntil = st.last
		}
		st.fetched = true
		l.active[id] = true
		go func(svc docker.Service, o docker.LogOptions) {
			err := cli.StreamLogs(l.ctx, svc, o, func(line docker.LogLine) bool {
				return l.send(streamEvent{line: &line})
			})
			if l.ctx.Err() == nil && err != nil {
				l.send(streamEvent{failed: err})
			}
			l.send(streamEvent{ended: svc.ContainerID})
		}(svc, opts)
	}
}

// add inserts a line in timestamp order (lines almost always arrive in order,
// so it searches from the end) and drops the oldest lines past the cap.
// It reports false for a line that repeats one already shown.
func (l *logSession) add(line docker.LogLine) bool {
	if st := l.containers[line.ContainerID]; st != nil {
		if !line.Time.IsZero() && !line.Time.After(st.skipUntil) {
			return false
		}
		if line.Time.After(st.last) {
			st.last = line.Time
		}
	}
	i := len(l.lines)
	for i > 0 && line.Time.Before(l.lines[i-1].Time) {
		i--
	}
	l.lines = append(l.lines, docker.LogLine{})
	copy(l.lines[i+1:], l.lines[i:])
	l.lines[i] = line
	if over := len(l.lines) - logCap; over > 0 {
		// Re-slicing shrinks the capacity, so the next append that needs room
		// copies just the kept lines into a new array: memory stays bounded.
		l.lines = l.lines[over:]
	}
	return true
}

func (l *logSession) colorIndex(service string) int {
	for i, s := range l.services {
		if s == service {
			return i
		}
	}
	l.services = append(l.services, service)
	return len(l.services) - 1
}

// statsSession streams CPU and memory for the selected project's running
// containers while the Stats tab is visible.
type statsSession struct {
	*session
	latest map[string]docker.Stats
}

func newStatsSession(project string) *statsSession {
	return &statsSession{session: newSession(project), latest: map[string]docker.Stats{}}
}

func (s *statsSession) sync(cli *docker.Client, p docker.Project) {
	for _, svc := range p.Services {
		id := svc.ContainerID
		if !svc.Running() || s.active[id] {
			continue
		}
		s.active[id] = true
		go func(id string) {
			cli.StreamStats(s.ctx, id, func(st docker.Stats) bool {
				return s.send(streamEvent{stats: &st})
			})
			s.send(streamEvent{ended: id})
		}(id)
	}
}

// stop is safe on a nil session (calling the embedded method through a nil
// wrapper would dereference it).
func (l *logSession) stop() {
	if l != nil {
		l.session.stop()
	}
}

func (s *statsSession) stop() {
	if s != nil {
		s.session.stop()
	}
}
