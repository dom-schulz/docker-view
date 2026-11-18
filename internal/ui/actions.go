package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dom-schulz/docker-view/internal/compose"
	"github.com/dom-schulz/docker-view/internal/config"
	"github.com/dom-schulz/docker-view/internal/docker"
)

const actionTimeout = 10 * time.Minute

func ref(p docker.Project) compose.Ref {
	return compose.Ref{Project: p.Name, WorkingDir: p.WorkingDir, ConfigFiles: p.ConfigFiles}
}

// target is what an action applies to: the selected service when the
// services list has focus, otherwise the whole project.
func (m Model) target() (docker.Project, *docker.Service, bool) {
	p, ok := m.project()
	if !ok {
		return p, nil, false
	}
	if m.focus == focusServices {
		if s, ok := m.service(); ok {
			return p, &s, true
		}
	}
	return p, nil, true
}

func targetName(p docker.Project, s *docker.Service) string {
	if s != nil {
		return p.Name + "/" + s.Name
	}
	return p.Name
}

// composeAction runs `docker compose <verb>` on the target. Containers
// outside compose (the Other group) get the matching plain docker command.
func (m *Model) composeAction(name string, verb ...string) tea.Cmd {
	p, s, ok := m.target()
	if !ok {
		return nil
	}
	desc := name + " " + targetName(p, s)
	if p.IsOther() {
		if s == nil {
			return m.setStatus("Other isn't a compose project: select one of its containers (Tab)", true)
		}
		dverb := map[string]string{"up": "start", "restart": "restart", "stop": "stop", "down": "rm"}[name]
		args := []string{dverb, s.ContainerID}
		if name == "down" {
			args = []string{"rm", "-f", s.ContainerID}
		}
		m.setBusy(desc + "…")
		return runCmd(desc, "docker", args...)
	}
	if s != nil && s.Service != "" {
		verb = append(verb, s.Service)
	}
	b := m.opt.Compose
	r := ref(p)
	m.setBusy(compose.Quote(b.Command(r, verb...)))
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
		defer cancel()
		out, err := b.Run(ctx, r, verb...)
		return actionMsg{desc: desc, out: out, err: err}
	}
}

func runCmd(desc, bin string, args ...string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
		defer cancel()
		out, err := exec.CommandContext(ctx, bin, args...).CombinedOutput()
		return actionMsg{desc: desc, out: string(out), err: err}
	}
}

func (m *Model) askDown() tea.Cmd {
	p, s, ok := m.target()
	if !ok {
		return nil
	}
	what := "removes the project's containers; it drops off the list and its logs go with it"
	if s != nil {
		what = "removes this service's container"
	}
	m.confirm = &confirmation{
		title: "Down " + targetName(p, s) + "?",
		lines: []string{what + ".", "Use s (stop) instead to keep containers and logs."},
		choices: []choice{{key: "y", label: "down", run: func(m *Model) tea.Cmd {
			return m.composeAction("down", "down")
		}}},
	}
	return nil
}

// askCleanup offers to `down` every project with nothing running.
func (m *Model) askCleanup() tea.Cmd {
	var stopped []docker.Project
	for _, p := range m.all {
		if p.Running == 0 && !p.IsOther() {
			stopped = append(stopped, p)
		}
	}
	if len(stopped) == 0 {
		return m.setStatus("nothing to clean up: every project has something running", false)
	}
	lines := make([]string, 0, len(stopped)+2)
	for _, p := range stopped {
		lines = append(lines, fmt.Sprintf("  ○ %s  (%d container%s)", p.Name, p.Total, plural(p.Total)))
	}
	run := func(volumes bool) func(*Model) tea.Cmd {
		return func(m *Model) tea.Cmd { return m.cleanup(stopped, volumes) }
	}
	c := &confirmation{
		title: fmt.Sprintf("Clean up %d stopped project%s?", len(stopped), plural(len(stopped))),
		lines: lines,
	}
	switch m.opt.Config.Cleanup.Volumes {
	case config.VolumesKeep:
		c.choices = []choice{{"y", "down", run(false)}}
	case config.VolumesRemove:
		c.lines = append(c.lines, "", "Volumes are removed too (cleanup.volumes = remove).")
		c.choices = []choice{{"y", "down -v", run(true)}}
	default:
		c.choices = []choice{{"y", "down", run(false)}, {"v", "down -v (also deletes volumes)", run(true)}}
	}
	m.confirm = c
	return nil
}

func (m *Model) cleanup(ps []docker.Project, volumes bool) tea.Cmd {
	verb := []string{"down"}
	if volumes {
		verb = append(verb, "-v")
	}
	b := m.opt.Compose
	m.setBusy(fmt.Sprintf("cleaning up %d project%s…", len(ps), plural(len(ps))))
	return func() tea.Msg {
		var failed []string
		var lastErr error
		var lastOut string
		for _, p := range ps {
			ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
			out, err := b.Run(ctx, ref(p), verb...)
			cancel()
			if err != nil {
				failed = append(failed, p.Name)
				lastErr, lastOut = err, out
			}
		}
		desc := fmt.Sprintf("clean up of %d project%s", len(ps), plural(len(ps)))
		if lastErr != nil {
			return actionMsg{desc: desc, out: strings.Join(failed, ", ") + ": " + lastLine(lastOut), err: lastErr}
		}
		return actionMsg{desc: desc}
	}
}

type execDoneMsg struct {
	what string
	err  error
}

func (m *Model) execService() tea.Cmd {
	s, ok := m.service()
	if !ok || m.focus != focusServices {
		return m.setStatus("select a container first (Tab to the containers list)", true)
	}
	if !s.Running() {
		return m.setStatus(s.Name+" isn't running", true)
	}
	cmd := exec.Command("docker", "exec", "-it", s.ContainerID, "sh", "-c",
		"if command -v bash >/dev/null 2>&1; then exec bash; else exec sh; fi")
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return execDoneMsg{what: "shell in " + s.Name, err: err} })
}

// projectShell hands the terminal to a login shell in the project's folder.
func (m *Model) projectShell() tea.Cmd {
	p, ok := m.project()
	if !ok {
		return nil
	}
	if p.WorkingDir == "" {
		return m.setStatus(p.Name+" has no working directory label", true)
	}
	if _, err := os.Stat(p.WorkingDir); err != nil {
		return m.setStatus(p.WorkingDir+" doesn't exist any more", true)
	}
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "/bin/sh"
	}
	c := exec.Command(sh, "-l")
	c.Dir = p.WorkingDir
	c.Env = append(os.Environ(), "DKRV_PROJECT="+p.Name, "PWD="+p.WorkingDir)
	sc := &shellCmd{
		cmd:    c,
		banner: Banner(p.Name, p.WorkingDir),
		title:  p.Name + " — shell",
	}
	return tea.Exec(sc, func(err error) tea.Msg { return execDoneMsg{what: "shell", err: err} })
}

// Banner is the line printed when the project shell opens.
func Banner(project, dir string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if dir == home {
			dir = "~"
		} else if strings.HasPrefix(dir, home+"/") {
			dir = "~" + dir[len(home):]
		}
	}
	return fmt.Sprintf("── %s · %s · type exit to return ──", project, dir)
}

// shellCmd wraps the project shell so the banner and window title are
// written to the real terminal around it.
type shellCmd struct {
	cmd           *exec.Cmd
	banner, title string
}

func (s *shellCmd) SetStdin(r io.Reader)  { s.cmd.Stdin = r }
func (s *shellCmd) SetStdout(w io.Writer) { s.cmd.Stdout = w }
func (s *shellCmd) SetStderr(w io.Writer) { s.cmd.Stderr = w }

func (s *shellCmd) Run() error {
	out := s.cmd.Stdout
	if out == nil {
		out = os.Stdout
	}
	// The shell gets normal wheel behaviour (e.g. for less) while it runs.
	fmt.Fprintf(out, "%s\x1b]2;%s\x07\x1b[2m%s\x1b[0m\r\n", WheelOn, s.title, s.banner)
	err := s.cmd.Run()
	fmt.Fprintf(out, "%s\x1b]2;%s\x07", WheelOff, windowTitle)
	// The shell's own exit status (e.g. from the last command) isn't an error.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return nil
	}
	return err
}

// ghosttyWindow opens a new Ghostty window in the project's folder and
// leaves dkrv running.
func (m *Model) ghosttyWindow() tea.Cmd {
	p, ok := m.project()
	if !ok {
		return nil
	}
	if p.WorkingDir == "" {
		return m.setStatus(p.Name+" has no working directory label", true)
	}
	if _, err := os.Stat(p.WorkingDir); err != nil {
		return m.setStatus(p.WorkingDir+" doesn't exist any more", true)
	}
	var c *exec.Cmd
	if runtime.GOOS == "darwin" {
		c = exec.Command("open", "-na", "Ghostty", "--args", "--working-directory="+p.WorkingDir)
	} else {
		c = exec.Command("ghostty", "--working-directory="+p.WorkingDir)
		detach(c)
	}
	c.Dir = p.WorkingDir
	if err := c.Start(); err != nil {
		return m.setStatus("ghostty: "+err.Error(), true)
	}
	go c.Wait()
	return m.setStatus("opened Ghostty in "+p.WorkingDir, false)
}

func (m Model) handleExecDone(msg execDoneMsg) (Model, tea.Cmd) {
	cmds := []tea.Cmd{tea.SetWindowTitle(windowTitle), m.list()}
	if msg.err != nil {
		cmds = append(cmds, m.setStatus(msg.what+": "+msg.err.Error(), true))
	}
	return m, tea.Batch(cmds...)
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// Alternate scroll mode (DECSET 1007): when on, the terminal sends the mouse
// wheel to full-screen apps as arrow keys. dkrv turns it off so the wheel
// doesn't move the selection or scroll the logs.
const (
	WheelOff = "\x1b[?1007l"
	WheelOn  = "\x1b[?1007h"
)
