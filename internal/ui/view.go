package ui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/docker/go-units"

	"github.com/dom-schulz/docker-view/internal/compose"
)

// Only the terminal's 16 ANSI colors, so dkrv follows the terminal theme.
var (
	cFocus  = lipgloss.Color("6")
	cDim    = lipgloss.Color("8")
	cGreen  = lipgloss.Color("2")
	cYellow = lipgloss.Color("3")
	cRed    = lipgloss.Color("1")

	sDim      = lipgloss.NewStyle().Foreground(cDim)
	sBold     = lipgloss.NewStyle().Bold(true)
	sSelected = lipgloss.NewStyle().Reverse(true)
	sSelBlur  = lipgloss.NewStyle().Bold(true).Underline(true)
	sGreen    = lipgloss.NewStyle().Foreground(cGreen)
	sYellow   = lipgloss.NewStyle().Foreground(cYellow)
	sRed      = lipgloss.NewStyle().Foreground(cRed)
	sTabOn    = lipgloss.NewStyle().Bold(true).Foreground(cFocus)

	serviceColors = []string{"6", "5", "3", "2", "4", "1", "14", "13", "11", "10", "12", "9"}
)

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	if !m.loaded {
		if m.daemonErr != nil {
			return m.errorScreen()
		}
		return center(m.width, m.height, sDim.Render("connecting to Docker…"))
	}

	lw := m.leftWidth()
	rw := m.width - lw
	ch := m.height - 1
	ph := m.projectsHeight()

	left := lipgloss.JoinVertical(lipgloss.Left,
		m.projectsBox(lw, ph),
		m.servicesBox(lw, ch-ph),
	)
	right := m.rightBox(rw, ch)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, right) + "\n" + m.bottomLine()
}

func (m Model) leftWidth() int {
	return clamp(m.width*32/100, min(28, m.width/2), 46)
}

func (m Model) projectsHeight() int {
	ch := m.height - 1
	want := len(m.visible) + 2
	if m.filtering || m.filter.Value() != "" {
		want++
	}
	return clamp(want, min(5, ch), max(5, ch*55/100))
}

// logHeight is the number of log lines that fit in the right pane.
func (m Model) logHeight() int {
	return max(1, m.height-1-2)
}

func (m Model) errorScreen() string {
	msg := []string{
		sRed.Bold(true).Render("Can't reach the Docker daemon"),
		"",
		"at " + m.opt.Client.Host(),
		sDim.Render(ansi.Truncate(m.daemonErr.Error(), m.width-4, "…")),
		"",
		"Start Docker (or set DOCKER_HOST). Retrying every few seconds…",
		sDim.Render("q to quit"),
	}
	return center(m.width, m.height, lipgloss.JoinVertical(lipgloss.Center, msg...))
}

func (m Model) projectsBox(w, h int) string {
	inner := w - 4
	var lines []string
	if m.filtering || m.filter.Value() != "" {
		m.filter.Width = inner - 3
		lines = append(lines, m.filter.View())
	}
	rows := h - 2 - len(lines)
	if len(m.visible) == 0 {
		if len(m.all) == 0 {
			lines = append(lines, sDim.Render("no compose projects"), sDim.Render("docker compose up to start one"))
		} else {
			lines = append(lines, sDim.Render("no matches"))
		}
	}
	start := scrollStart(m.selProject, len(m.visible), rows)
	for i := start; i < len(m.visible) && i < start+rows; i++ {
		p := m.visible[i]
		num := " "
		if i < 9 {
			num = fmt.Sprint(i + 1)
		}
		dot := sDim.Render("○")
		switch {
		case p.Running > 0 && p.Running == p.Total:
			dot = sGreen.Render("●")
		case p.Running > 0:
			dot = sYellow.Render("●")
		}
		count := fmt.Sprintf("%d/%d", p.Running, p.Total)
		nameW := inner - 4 - len(count) - 1
		name := pad(ansi.Truncate(p.Name, nameW, "…"), nameW)
		row := fmt.Sprintf("%s %s %s %s", sDim.Render(num), dot, m.selStyle(i == m.selProject, focusProjects).Render(name), count)
		lines = append(lines, row)
	}
	title := "Projects"
	if m.hideStopped {
		title += sDim.Render(" (running)")
	}
	return box(title, "", lines, w, h, m.focus == focusProjects)
}

func (m Model) selStyle(selected bool, f focus) lipgloss.Style {
	if !selected {
		return lipgloss.NewStyle()
	}
	if m.focus == f {
		return sSelected
	}
	return sSelBlur
}

func (m Model) servicesBox(w, h int) string {
	p, ok := m.project()
	title := "Services"
	if ok {
		title += ": " + p.Name
	}
	inner := w - 4
	var lines []string
	if ok {
		rows := h - 2
		nameW := 4
		for _, s := range p.Services {
			nameW = max(nameW, ansi.StringWidth(s.Name))
		}
		nameW = min(nameW, inner/2)
		start := scrollStart(m.selService, len(p.Services), rows)
		for i := start; i < len(p.Services) && i < start+rows; i++ {
			s := p.Services[i]
			dot := sDim.Render("○")
			switch s.State {
			case "running":
				dot = sGreen.Render("●")
			case "paused", "restarting":
				dot = sYellow.Render("●")
			case "dead":
				dot = sRed.Render("●")
			}
			mark := " "
			if s.Name == m.serviceFilter {
				mark = sTabOn.Render("▸")
			}
			name := pad(ansi.Truncate(s.Name, nameW, "…"), nameW)
			extra := s.Ports
			if !s.Running() {
				extra = s.Status
			}
			row := fmt.Sprintf("%s%s %s %s", mark, dot, m.selStyle(i == m.selService, focusServices).Render(name), sDim.Render(extra))
			lines = append(lines, row)
		}
	}
	return box(title, "", lines, w, h, m.focus == focusServices)
}

func (m Model) rightBox(w, h int) string {
	var title string
	for i, n := range tabNames {
		if i > 0 {
			title += sDim.Render(" ─ ")
		}
		if tab(i) == m.tab && !m.help && m.confirm == nil {
			title += sTabOn.Render(n)
		} else {
			title += n
		}
	}
	inner := w - 4
	rows := h - 2
	var lines []string
	meta := ""
	switch {
	case m.confirm != nil:
		title = sYellow.Bold(true).Render(m.confirm.title)
		lines = m.confirmLines()
	case m.help:
		title = sTabOn.Render("Help")
		lines = helpLines(inner)
	case m.tab == tabLogs:
		lines, meta = m.logLines(inner, rows)
	case m.tab == tabStats:
		lines = m.statsLines(inner)
	case m.tab == tabConfig:
		lines = m.configLines(inner)
	}
	return box(title, meta, lines, w, h, m.focus == focusRight)
}

func (m Model) logLines(inner, rows int) ([]string, string) {
	all := m.visibleLogs()
	meta := sGreen.Render("● following")
	if m.scroll > 0 {
		meta = sYellow.Render(fmt.Sprintf("↑ %d · G to follow", m.scroll))
	}
	if m.serviceFilter != "" {
		meta = sTabOn.Render(m.serviceFilter) + sDim.Render(" (esc: all) ") + meta
	}
	if len(all) == 0 {
		if m.logs == nil {
			return nil, ""
		}
		return []string{sDim.Render("no logs yet")}, meta
	}
	end := len(all) - m.scroll
	start := max(0, end-rows)

	nameW := 0
	for _, s := range m.logs.services {
		if m.serviceFilter == "" || s == m.serviceFilter {
			nameW = max(nameW, ansi.StringWidth(s))
		}
	}
	nameW = min(nameW, max(6, inner/4))
	out := make([]string, 0, rows)
	for _, l := range all[start:end] {
		color := serviceColors[m.logs.colorIndexOf(l.Service)%len(serviceColors)]
		prefix := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(pad(ansi.Truncate(l.Service, nameW, "…"), nameW))
		sep := sDim.Render(" │ ")
		if l.Stderr {
			sep = sRed.Render(" │ ")
		}
		text := ansi.Truncate(l.Text, inner-nameW-3, "…")
		out = append(out, prefix+sep+text+"\x1b[0m")
	}
	return out, meta
}

func (m Model) statsLines(inner int) []string {
	p, ok := m.project()
	if !ok {
		return nil
	}
	nameW := 7
	for _, s := range p.Services {
		nameW = max(nameW, ansi.StringWidth(s.Name))
	}
	nameW = min(nameW, inner/3)
	row := func(a, b, c, d, e string) string {
		return fmt.Sprintf("%s  %-10s %8s  %21s  %6s", pad(ansi.Truncate(a, nameW, "…"), nameW), b, c, d, e)
	}
	lines := []string{sBold.Render(row("SERVICE", "STATE", "CPU", "MEMORY", "MEM %")), ""}
	for _, s := range p.Services {
		cpu, mem, memPct := "–", "–", "–"
		if m.stats != nil {
			if st, ok := m.stats.latest[s.ContainerID]; ok {
				cpu = fmt.Sprintf("%.1f%%", st.CPUPercent)
				mem = units.BytesSize(float64(st.MemUsage)) + " / " + units.BytesSize(float64(st.MemLimit))
				memPct = fmt.Sprintf("%.1f%%", st.MemPercent())
			} else if s.Running() {
				cpu = "…"
			}
		}
		lines = append(lines, row(s.Name, s.State, cpu, mem, memPct))
	}
	return lines
}

func (m Model) configLines(inner int) []string {
	p, ok := m.project()
	if !ok {
		return nil
	}
	kv := func(k, v string) string { return sDim.Render(pad(k, 13)) + v }
	lines := []string{
		kv("Project", sBold.Render(p.Name)),
		kv("Containers", fmt.Sprintf("%d running of %d", p.Running, p.Total)),
	}
	if !p.IsOther() {
		dir := p.WorkingDir
		if dir == "" {
			dir = sDim.Render("(no label)")
		}
		lines = append(lines, kv("Directory", dir))
		for i, f := range p.ConfigFiles {
			k := ""
			if i == 0 {
				k = "Compose files"
			}
			lines = append(lines, kv(k, f))
		}
		lines = append(lines, "", kv("Commands run", ""),
			"  "+compose.Quote(m.opt.Compose.Command(ref(p), "<verb>")),
			sDim.Render("  Flags such as --profile aren't recorded in labels, so they aren't reproduced."))
	}
	lines = append(lines, "",
		kv("dkrv config", m.opt.ConfigPath),
		kv("", "cleanup.volumes = "+m.opt.Config.Cleanup.Volumes),
		kv("Version", m.opt.Version),
	)
	return lines
}

func (m Model) confirmLines() []string {
	lines := append([]string{}, m.confirm.lines...)
	lines = append(lines, "")
	for _, c := range m.confirm.choices {
		lines = append(lines, "  "+sBold.Render(c.key)+"  "+c.label)
	}
	lines = append(lines, "  "+sBold.Render("n")+"  cancel")
	return lines
}

func helpLines(inner int) []string {
	lines := make([]string, 0, len(helpRows))
	for _, r := range helpRows {
		lines = append(lines, sBold.Render(pad(r[0], 16))+sDim.Render(pad(r[1], 17))+r[2])
	}
	return append(lines, "", sDim.Render("? or esc to close"))
}

func (m Model) bottomLine() string {
	var s string
	switch {
	case m.confirm != nil:
		var parts []string
		for _, c := range m.confirm.choices {
			parts = append(parts, c.key+" "+c.label)
		}
		s = sYellow.Render(m.confirm.title + "  " + strings.Join(parts, " · ") + " · n cancel")
	case m.status != "" && m.statusErr:
		s = sRed.Render("✗ " + m.status)
	case m.status != "" && m.statusBusy:
		s = sYellow.Render("… " + m.status)
	case m.status != "":
		s = sGreen.Render("✓ ") + m.status
	case m.daemonErr != nil:
		s = sRed.Render("✗ Docker: " + m.daemonErr.Error())
	default:
		s = sDim.Render(hints)
	}
	return " " + ansi.Truncate(s, m.width-2, "…")
}

// box draws a rounded-corner-free frame with a title in the top border, like
//
//	┌ Title ──────── meta ┐
func box(title, meta string, body []string, w, h int, focused bool) string {
	if w < 4 || h < 2 {
		return ""
	}
	bc := sDim
	if focused {
		bc = lipgloss.NewStyle().Foreground(cFocus)
	}
	inner := w - 2
	title = ansi.Truncate(title, max(0, inner-2), "…")
	top := bc.Render("┌") + " " + title + " "
	used := 1 + 1 + ansi.StringWidth(title) + 1
	if meta != "" {
		meta = " " + meta + " "
		if used+ansi.StringWidth(meta)+2 > w {
			meta = ""
		}
	}
	fill := w - used - ansi.StringWidth(meta) - 1
	top += bc.Render(strings.Repeat("─", max(0, fill))) + meta + bc.Render("┐")

	var b strings.Builder
	b.WriteString(top)
	side := bc.Render("│")
	for i := 0; i < h-2; i++ {
		line := ""
		if i < len(body) {
			line = ansi.Truncate(body[i], inner-2, "…")
		}
		b.WriteString("\n" + side + " " + pad(line, inner-2) + " " + side)
	}
	b.WriteString("\n" + bc.Render("└"+strings.Repeat("─", inner)+"┘"))
	return b.String()
}

func pad(s string, w int) string {
	if n := ansi.StringWidth(s); n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s
}

func center(w, h int, s string) string {
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, s)
}

// scrollStart keeps the selection on screen in a list of n rows.
func scrollStart(sel, n, rows int) int {
	if rows <= 0 || n <= rows {
		return 0
	}
	return clamp(sel-rows/2, 0, n-rows)
}

var (
	reCSI   = regexp.MustCompile(`\x1b\[[0-9;:?<=>]*[ -/]*[@-~]`)
	reOSC   = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`)
	reESC   = regexp.MustCompile(`\x1b[^\[\]]`)
	reCtrl  = regexp.MustCompile(`[\x00-\x08\x0b-\x1a\x1c-\x1f\x7f]`)
	tabRepl = strings.NewReplacer("\t", "    ")
)

// sanitize keeps color (SGR) sequences in a log line and strips everything
// else that could move the cursor or retitle the window.
func sanitize(s string) string {
	if strings.ContainsRune(s, '\r') {
		// Progress bars redraw with \r: keep what was drawn last.
		parts := strings.Split(s, "\r")
		for i := len(parts) - 1; i >= 0; i-- {
			if parts[i] != "" {
				s = parts[i]
				break
			}
		}
	}
	s = tabRepl.Replace(s)
	if !strings.ContainsAny(s, "\x1b\x00\x01\x02\x03\x04\x05\x06\x07\x08\x0b\x0c\x0e\x0f\x7f") {
		return s
	}
	s = reOSC.ReplaceAllString(s, "")
	s = reCSI.ReplaceAllStringFunc(s, func(seq string) string {
		if strings.HasSuffix(seq, "m") {
			return seq
		}
		return ""
	})
	s = reESC.ReplaceAllString(s, "")
	return reCtrl.ReplaceAllString(s, "")
}

func (l *logSession) colorIndexOf(service string) int {
	for i, s := range l.services {
		if s == service {
			return i
		}
	}
	return 0
}
