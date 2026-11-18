package ui

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/dom-schulz/docker-view/internal/docker"
)

func TestLogBufferOrdersAndCaps(t *testing.T) {
	l := newLogSession("p")
	defer l.stop()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// Two services whose tails arrive one after the other.
	for i := 0; i < 3; i++ {
		l.add(docker.LogLine{Time: base.Add(time.Duration(2*i) * time.Second), Service: "a", Text: fmt.Sprint("a", i)})
	}
	for i := 0; i < 3; i++ {
		l.add(docker.LogLine{Time: base.Add(time.Duration(2*i+1) * time.Second), Service: "b", Text: fmt.Sprint("b", i)})
	}
	var got string
	for _, ln := range l.lines {
		got += ln.Text + " "
	}
	if want := "a0 b0 a1 b1 a2 b2 "; got != want {
		t.Fatalf("order = %q, want %q", got, want)
	}

	for i := 0; i < logCap*3; i++ {
		l.add(docker.LogLine{Time: base.Add(time.Hour + time.Duration(i)*time.Millisecond), Service: "a"})
	}
	if len(l.lines) != logCap {
		t.Fatalf("len = %d, want cap %d", len(l.lines), logCap)
	}
	if c := cap(l.lines); c > 2*logCap {
		t.Errorf("backing array grew to %d", c)
	}
}

func TestLogBufferSkipsRepeatOnResume(t *testing.T) {
	l := newLogSession("p")
	defer l.stop()
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l.containers["c"] = &containerLog{fetched: true, last: at, skipUntil: at}
	if l.add(docker.LogLine{ContainerID: "c", Time: at, Text: "again"}) {
		t.Error("repeated line was kept")
	}
	if !l.add(docker.LogLine{ContainerID: "c", Time: at.Add(time.Millisecond), Text: "new"}) {
		t.Error("new line was dropped")
	}
}

func TestSanitize(t *testing.T) {
	for in, want := range map[string]string{
		"plain":                 "plain",
		"\x1b[32mgreen\x1b[0m":  "\x1b[32mgreen\x1b[0m",
		"\x1b[2Jclear\x1b[H":    "clear",
		"\x1b]2;title\x07text":  "text",
		"tab\there":             "tab    here",
		"10%\r50%\r100%":        "100%",
		"bell\x07 and \x00null": "bell and null",
	} {
		if got := sanitize(in); got != want {
			t.Errorf("sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBanner(t *testing.T) {
	home, _ := os.UserHomeDir()
	got := Banner("api-feature-x", home+"/projects/api-feature-x")
	if want := "── api-feature-x · ~/projects/api-feature-x · type exit to return ──"; got != want {
		t.Errorf("banner = %q, want %q", got, want)
	}
}

func TestBox(t *testing.T) {
	b := box("Projects", "", []string{"one"}, 20, 4, false)
	if got := len([]rune(stripANSI(b))); got != 20*4+3 {
		t.Errorf("box size = %d runes:\n%s", got, b)
	}
}

func stripANSI(s string) string { return reCSI.ReplaceAllString(s, "") }
