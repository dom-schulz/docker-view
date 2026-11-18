// Package compose builds and runs docker compose commands for a project,
// rebuilt from the labels compose puts on its containers.
package compose

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
)

// Ref identifies a compose project the way its container labels describe it.
type Ref struct {
	Project     string
	WorkingDir  string
	ConfigFiles []string
}

// Builder turns a Ref and a verb into a docker compose command line.
type Builder struct {
	// Bin and Prefix form the command: "docker" + ["compose"], or
	// "docker-compose" with no prefix when the plugin isn't installed.
	Bin    string
	Prefix []string
	// Exists reports whether a path exists. Paths that are gone (a deleted
	// worktree) are left out so `down` still works by project name alone.
	Exists func(string) bool
}

// Detect picks `docker compose`, falling back to `docker-compose`.
func Detect() Builder {
	b := Builder{Bin: "docker", Prefix: []string{"compose"}, Exists: pathExists}
	if err := exec.Command("docker", "compose", "version").Run(); err != nil {
		if p, lerr := exec.LookPath("docker-compose"); lerr == nil {
			b.Bin, b.Prefix = p, nil
		}
	}
	return b
}

// Args returns the arguments (without Bin) for running verb on ref:
//
//	compose -p <project> --project-directory <dir> -f <file>... <verb...>
//
// It never relies on the current directory.
func (b Builder) Args(ref Ref, verb ...string) []string {
	exists := b.Exists
	if exists == nil {
		exists = pathExists
	}
	args := append([]string{}, b.Prefix...)
	args = append(args, "-p", ref.Project)
	if ref.WorkingDir != "" && exists(ref.WorkingDir) {
		args = append(args, "--project-directory", ref.WorkingDir)
	}
	for _, f := range ref.ConfigFiles {
		if exists(f) {
			args = append(args, "-f", f)
		}
	}
	return append(args, verb...)
}

// Command returns the full command line, for display.
func (b Builder) Command(ref Ref, verb ...string) []string {
	return append([]string{b.Bin}, b.Args(ref, verb...)...)
}

// Run runs verb on ref and returns its combined output.
func (b Builder) Run(ctx context.Context, ref Ref, verb ...string) (string, error) {
	cmd := exec.CommandContext(ctx, b.Bin, b.Args(ref, verb...)...)
	if ref.WorkingDir != "" && b.exists(ref.WorkingDir) {
		cmd.Dir = ref.WorkingDir
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return strings.TrimSpace(out.String()), err
}

func (b Builder) exists(p string) bool {
	if b.Exists == nil {
		return pathExists(p)
	}
	return b.Exists(p)
}

func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// Quote renders a command line for display, quoting arguments with spaces.
func Quote(args []string) string {
	out := make([]string, len(args))
	for i, a := range args {
		if a == "" || strings.ContainsAny(a, " \t'\"$") {
			a = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		}
		out[i] = a
	}
	return strings.Join(out, " ")
}
