package ui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// helpRows is the key table shown by ?.
var helpRows = [][3]string{
	{"↑ ↓  j k", "any list", "move the selection"},
	{"1–9", "anywhere", "jump to project 1–9"},
	{"Tab  Shift+Tab", "anywhere", "move focus: projects, services, right pane"},
	{"[  ]", "anywhere", "switch tab: Logs, Stats, Config"},
	{"Enter", "services", "logs for that service only (Esc: all)"},
	{"/", "projects", "filter projects by name"},
	{"a", "projects", "show or hide stopped projects"},
	{"t", "project", "shell in the project's folder; exit returns"},
	{"T", "project", "new Ghostty window in the project's folder"},
	{"u  d", "project/service", "compose up -d / down (down asks first)"},
	{"r  s", "project/service", "restart / stop"},
	{"e", "services", "shell into the service's container"},
	{"C", "anywhere", "clean up every project with nothing running"},
	{"PgUp PgDn  G", "logs", "scroll / jump back to following"},
	{"?  q", "anywhere", "help / quit"},
}

const hints = "1-9 jump · ↑↓ move · tab focus · enter service logs · u up · d down · t shell · C clean up · ? help · q quit"

func (m Model) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := k.String()
	if key == "ctrl+c" {
		return m, m.quit()
	}

	if m.confirm != nil {
		for _, c := range m.confirm.choices {
			if key == c.key {
				m.confirm = nil
				return m, c.run(&m)
			}
		}
		if key == "esc" || key == "n" || key == "q" {
			m.confirm = nil
			return m, m.setStatus("cancelled", false)
		}
		return m, nil
	}

	if m.filtering {
		switch key {
		case "esc":
			m.filtering = false
			m.filter.Blur()
			m.filter.SetValue("")
			return m, m.refreshVisible()
		case "enter":
			m.filtering = false
			m.filter.Blur()
			return m, nil
		case "up", "down":
			// fall through to list movement below
		default:
			var cmd tea.Cmd
			m.filter, cmd = m.filter.Update(k)
			return m, tea.Batch(cmd, m.refreshVisible())
		}
	}

	if m.help {
		switch key {
		case "?", "esc", "q":
			m.help = false
		}
		return m, nil
	}

	if m.daemonErr != nil && !m.loaded {
		if key == "q" {
			return m, m.quit()
		}
		return m, nil
	}

	switch key {
	case "q":
		return m, m.quit()
	case "?":
		m.help = true
		return m, nil

	case "tab":
		m.focus = (m.focus + 1) % 3
		return m, nil
	case "shift+tab":
		m.focus = (m.focus + 2) % 3
		return m, nil
	case "]":
		return m, m.setTab((m.tab + 1) % tabCount)
	case "[":
		return m, m.setTab((m.tab + tabCount - 1) % tabCount)

	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		return m, m.selectProject(int(key[0] - '1'))

	case "up", "k":
		return m, m.move(-1)
	case "down", "j":
		return m, m.move(1)
	case "home":
		return m, m.move(-1 << 20)
	case "end":
		return m, m.move(1 << 20)

	case "pgup", "ctrl+u", "ctrl+b":
		m.scroll += max(1, m.logHeight()-1)
		m.clampScroll()
		return m, nil
	case "pgdown", "ctrl+d", "ctrl+f":
		m.scroll -= max(1, m.logHeight()-1)
		m.clampScroll()
		return m, nil
	case "G":
		m.scroll = 0
		return m, nil
	case "g":
		m.scroll = len(m.visibleLogs())
		m.clampScroll()
		return m, nil

	case "enter":
		switch m.focus {
		case focusProjects:
			m.focus = focusServices
		case focusServices:
			if s, ok := m.service(); ok {
				m.serviceFilter = s.Name
				m.scroll = 0
				m.tab = tabLogs
			}
		}
		return m, nil
	case "esc":
		if m.serviceFilter != "" {
			m.serviceFilter = ""
			m.scroll = 0
			return m, nil
		}
		if m.filter.Value() != "" {
			m.filter.SetValue("")
			return m, m.refreshVisible()
		}
		return m, nil

	case "/":
		m.filtering = true
		m.focus = focusProjects
		return m, m.filter.Focus()
	case "a":
		m.hideStopped = !m.hideStopped
		msg := "showing stopped projects"
		if m.hideStopped {
			msg = "hiding stopped projects"
		}
		return m, tea.Batch(m.setStatus(msg, false), m.refreshVisible())

	case "u":
		return m, m.composeAction("up", "up", "-d")
	case "r":
		return m, m.composeAction("restart", "restart")
	case "s":
		return m, m.composeAction("stop", "stop")
	case "d":
		return m, m.askDown()
	case "e":
		return m, m.execService()
	case "t":
		return m, m.projectShell()
	case "T":
		return m, m.ghosttyWindow()
	case "C":
		return m, m.askCleanup()
	}
	return m, nil
}

// move moves the selection in the focused list, or scrolls logs when the
// right pane has focus.
func (m *Model) move(d int) tea.Cmd {
	switch m.focus {
	case focusProjects:
		i := clamp(m.selProject+d, 0, len(m.visible)-1)
		return m.selectProject(i)
	case focusServices:
		if p, ok := m.project(); ok {
			m.selService = clamp(m.selService+d, 0, len(p.Services)-1)
		}
	case focusRight:
		if m.tab == tabLogs {
			m.scroll -= d
			m.clampScroll()
		}
	}
	return nil
}

func (m *Model) setTab(t tab) tea.Cmd {
	m.tab = t
	return m.selectionChanged()
}

func (m *Model) quit() tea.Cmd {
	m.logs.stop()
	m.stats.stop()
	return tea.Quit
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	return min(max(v, lo), hi)
}
