//go:build unix

package ui

import (
	"os/exec"
	"syscall"
)

// detach starts c in its own session so it outlives dkrv and doesn't
// receive the terminal's signals.
func detach(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
