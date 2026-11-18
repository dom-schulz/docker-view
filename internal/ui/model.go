// Package ui is dkrv's Bubble Tea interface.
package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/dom-schulz/docker-view/internal/compose"
	"github.com/dom-schulz/docker-view/internal/config"
	"github.com/dom-schulz/docker-view/internal/docker"
)

type focus int

const (
	focusProjects focus = iota
	focusServices
	focusRight
)

type tab int

const (
	tabLogs tab = iota
	tabStats
	tabConfig
	tabCount
)

var tabNames = [...]string{"Logs", "Stats", "Config"}

const (
	debounceDelay = 200 * time.Millisecond
	safetyRelist  = 30 * time.Second
	retryDelay    = 2 * time.Second
	statusTimeout = 6 * time.Second
	windowTitle   = "dkrv"
)

// Options are what main passes in.
type Options struct {
	Client     *docker.Client
	Config     config.Config
	ConfigPath string
	ConfigErr  error
	Compose    compose.Builder
	Version    string
}

// Model is the whole UI state.
type Model struct {
	opt Options

	width, height int

	all     []docker.Project // every project from the last listing
	visible []docker.Project // after the name filter and show/hide stopped
	loaded  bool

	selName    string // selected project name, kept across re-lists
	selProject int
	selService int
	focus      focus
	tab        tab

	filter      textinput.Model
	filtering   bool
	hideStopped bool

	logs          *logSession
	serviceFilter string // Enter on a service narrows logs to it
	scroll        int    // lines scrolled up from the bottom; 0 = following

	stats *statsSession

	status     string
	statusErr  bool
	statusBusy bool
	statusID   int

	confirm *confirmation
	help    bool

	daemonErr    error
	retryPending bool
	debounceID   int
	watch        <-chan struct{}
}

type confirmation struct {
	title   string
	lines   []string
	choices []choice
}

type choice struct {
	key   string
	label string
	run   func(m *Model) tea.Cmd
}

// Messages.
type (
	projectsMsg struct {
		projects []docker.Project
		err      error
	}
	watchMsg    struct{}
	debounceMsg struct{ id int }
	safetyMsg   struct{}
	retryMsg    struct{}
	actionMsg   struct {
		desc string
		out  string
		err  error
	}
	clearStatusMsg struct{ id int }
)

// New builds the model and starts watching container events.
func New(o Options) Model {
	ti := textinput.New()
	ti.Prompt = "/ "
	ti.Placeholder = "filter projects"
	ti.CharLimit = 64
	m := Model{opt: o, filter: ti}
	m.watch = o.Client.Watch(context.Background())
	if o.ConfigErr != nil {
		m.setStatus(fmt.Sprintf("config: %v (using defaults)", o.ConfigErr), true)
	}
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tea.SetWindowTitle(windowTitle),
		m.list(),
		m.waitWatch(),
		tea.Tick(safetyRelist, func(time.Time) tea.Msg { return safetyMsg{} }),
	)
}

func (m Model) list() tea.Cmd {
	cli := m.opt.Client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		ps, err := cli.Projects(ctx)
		return projectsMsg{projects: ps, err: err}
	}
}

func (m Model) waitWatch() tea.Cmd {
	ch := m.watch
	return func() tea.Msg {
		<-ch
		return watchMsg{}
	}
}

func (m *Model) setStatus(s string, isErr bool) tea.Cmd {
	m.status, m.statusErr, m.statusBusy = s, isErr, false
	m.statusID++
	if isErr || s == "" {
		return nil
	}
	id := m.statusID
	return tea.Tick(statusTimeout, func(time.Time) tea.Msg { return clearStatusMsg{id} })
}

func (m *Model) setBusy(s string) {
	m.status, m.statusErr, m.statusBusy = s, false, true
	m.statusID++
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clampScroll()
		return m, nil

	case projectsMsg:
		if msg.err != nil {
			m.daemonErr = msg.err
			if !m.retryPending {
				m.retryPending = true
				return m, tea.Tick(retryDelay, func(time.Time) tea.Msg { return retryMsg{} })
			}
			return m, nil
		}
		m.daemonErr = nil
		m.loaded = true
		m.all = msg.projects
		return m, m.refreshVisible()

	case retryMsg:
		m.retryPending = false
		return m, m.list()

	case watchMsg:
		m.debounceID++
		id := m.debounceID
		return m, tea.Batch(
			m.waitWatch(),
			tea.Tick(debounceDelay, func(time.Time) tea.Msg { return debounceMsg{id} }),
		)

	case debounceMsg:
		if msg.id != m.debounceID {
			return m, nil
		}
		return m, m.list()

	case safetyMsg:
		return m, tea.Batch(m.list(), tea.Tick(safetyRelist, func(time.Time) tea.Msg { return safetyMsg{} }))

	case streamBatchMsg:
		return m, m.handleStream(msg)

	case actionMsg:
		var cmd tea.Cmd
		if msg.err != nil {
			text := lastLine(msg.out)
			if text == "" {
				text = msg.err.Error()
			}
			cmd = m.setStatus(msg.desc+" failed: "+text, true)
		} else {
			text := msg.desc + " done"
			if l := lastLine(msg.out); l != "" {
				text += " · " + l
			}
			cmd = m.setStatus(text, false)
		}
		return m, tea.Batch(cmd, m.list())

	case execDoneMsg:
		return m.handleExecDone(msg)

	case clearStatusMsg:
		if msg.id == m.statusID {
			m.status = ""
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	if m.filtering {
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		return m, cmd
	}
	return m, nil
}

// refreshVisible applies the filter, keeps the selection on the same project
// name, and (re)starts streams for the selected project.
func (m *Model) refreshVisible() tea.Cmd {
	q := strings.ToLower(strings.TrimSpace(m.filter.Value()))
	m.visible = m.visible[:0:0]
	for _, p := range m.all {
		if m.hideStopped && p.Running == 0 {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(p.Name), q) {
			continue
		}
		m.visible = append(m.visible, p)
	}
	m.selProject = 0
	for i, p := range m.visible {
		if p.Name == m.selName {
			m.selProject = i
			break
		}
	}
	return m.selectionChanged()
}

func (m *Model) selectProject(i int) tea.Cmd {
	if i < 0 || i >= len(m.visible) {
		return nil
	}
	if i != m.selProject {
		m.selService = 0
	}
	m.selProject = i
	return m.selectionChanged()
}

// selectionChanged starts new streams when the selected project changed, or
// tops up the current ones (new or restarted containers) when it didn't.
func (m *Model) selectionChanged() tea.Cmd {
	p, ok := m.project()
	if !ok {
		m.selName = ""
		m.logs.stop()
		m.stats.stop()
		m.logs, m.stats = nil, nil
		return nil
	}
	m.selName = p.Name
	if m.selService >= len(p.Services) {
		m.selService = max(0, len(p.Services)-1)
	}

	var cmds []tea.Cmd
	if m.logs == nil || m.logs.project != p.Name {
		m.logs.stop()
		m.logs = newLogSession(p.Name)
		m.serviceFilter = ""
		m.scroll = 0
		cmds = append(cmds, m.logs.wait())
	}
	m.logs.sync(m.opt.Client, p)

	if m.tab == tabStats {
		if m.stats == nil || m.stats.project != p.Name {
			m.stats.stop()
			m.stats = newStatsSession(p.Name)
			cmds = append(cmds, m.stats.wait())
		}
		m.stats.sync(m.opt.Client, p)
	} else if m.stats != nil {
		m.stats.stop()
		m.stats = nil
	}
	return tea.Batch(cmds...)
}

func (m *Model) handleStream(msg streamBatchMsg) tea.Cmd {
	switch {
	case m.logs != nil && msg.gen == m.logs.gen:
		var errCmd tea.Cmd
		for _, ev := range msg.events {
			switch {
			case ev.line != nil:
				ev.line.Text = sanitize(ev.line.Text)
				if m.logs.add(*ev.line) && m.scroll > 0 && (m.serviceFilter == "" || ev.line.Service == m.serviceFilter) {
					m.scroll++ // keep the scrolled view where it is
				}
			case ev.ended != "":
				m.logs.active[ev.ended] = false
			case ev.failed != nil:
				errCmd = m.setStatus("logs: "+ev.failed.Error(), true)
			}
		}
		m.clampScroll()
		return tea.Batch(errCmd, m.logs.wait())
	case m.stats != nil && msg.gen == m.stats.gen:
		for _, ev := range msg.events {
			switch {
			case ev.stats != nil:
				m.stats.latest[ev.stats.ContainerID] = *ev.stats
			case ev.ended != "":
				m.stats.active[ev.ended] = false
				delete(m.stats.latest, ev.ended)
			}
		}
		return m.stats.wait()
	}
	return nil // a cancelled session; let its waiter lapse
}

func (m Model) project() (docker.Project, bool) {
	if m.selProject < 0 || m.selProject >= len(m.visible) {
		return docker.Project{}, false
	}
	return m.visible[m.selProject], true
}

func (m Model) service() (docker.Service, bool) {
	p, ok := m.project()
	if !ok || m.selService < 0 || m.selService >= len(p.Services) {
		return docker.Service{}, false
	}
	return p.Services[m.selService], true
}

// visibleLogs returns the log lines that pass the service filter.
func (m Model) visibleLogs() []docker.LogLine {
	if m.logs == nil {
		return nil
	}
	if m.serviceFilter == "" {
		return m.logs.lines
	}
	out := make([]docker.LogLine, 0, len(m.logs.lines)/2)
	for _, l := range m.logs.lines {
		if l.Service == m.serviceFilter {
			out = append(out, l)
		}
	}
	return out
}

func (m *Model) clampScroll() {
	maxScroll := len(m.visibleLogs()) - m.logHeight()
	if m.scroll > maxScroll {
		m.scroll = maxScroll
	}
	if m.scroll < 0 {
		m.scroll = 0
	}
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			return l
		}
	}
	return ""
}
