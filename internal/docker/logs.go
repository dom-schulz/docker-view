package docker

import (
	"bufio"
	"context"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
)

// LogLine is one line of container output.
type LogLine struct {
	Time        time.Time
	ContainerID string
	Service     string
	Text        string
	Stderr      bool
}

// LogOptions controls where a log stream starts.
type LogOptions struct {
	Tail   int       // lines of history to fetch; ignored when Since is set
	Since  time.Time // resume after this time (exclusive), e.g. after a restart
	Follow bool
}

// StreamLogs reads a container's logs and calls send for each line until the
// stream ends (the container stops, ctx is cancelled, or send returns false).
func (c *Client) StreamLogs(ctx context.Context, s Service, o LogOptions, send func(LogLine) bool) error {
	info, err := c.api.ContainerInspect(ctx, s.ContainerID)
	if err != nil {
		return err
	}
	opts := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     o.Follow,
		Timestamps: true,
	}
	if !o.Since.IsZero() {
		since := o.Since.Add(time.Nanosecond)
		opts.Since = strconv.FormatInt(since.Unix(), 10) + "." + leftPad(strconv.Itoa(since.Nanosecond()), 9)
	} else {
		opts.Tail = strconv.Itoa(o.Tail)
	}
	rc, err := c.api.ContainerLogs(ctx, s.ContainerID, opts)
	if err != nil {
		return err
	}
	defer rc.Close()
	go func() { <-ctx.Done(); rc.Close() }()

	var mu sync.Mutex
	stopped := false
	emit := func(l LogLine) bool {
		mu.Lock()
		defer mu.Unlock()
		if stopped {
			return false
		}
		if !send(l) {
			stopped = true
			rc.Close()
			return false
		}
		return true
	}

	if info.Config != nil && info.Config.Tty {
		scanLines(rc, s, false, emit)
		return ctx.Err()
	}

	// Without a TTY the daemon multiplexes stdout and stderr into one stream.
	outR, outW := io.Pipe()
	errR, errW := io.Pipe()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); scanLines(outR, s, false, emit); outR.CloseWithError(io.EOF) }()
	go func() { defer wg.Done(); scanLines(errR, s, true, emit); errR.CloseWithError(io.EOF) }()
	_, err = stdcopy.StdCopy(outW, errW, rc)
	outW.Close()
	errW.Close()
	wg.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func scanLines(r io.Reader, s Service, stderr bool, emit func(LogLine) bool) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		if !emit(ParseLogLine(sc.Text(), s, stderr)) {
			// Keep draining so the demuxer never blocks on a full pipe.
			io.Copy(io.Discard, r)
			return
		}
	}
	io.Copy(io.Discard, r)
}

// ParseLogLine splits the RFC 3339 timestamp the daemon prefixes to each
// line when Timestamps is set.
func ParseLogLine(raw string, s Service, stderr bool) LogLine {
	l := LogLine{ContainerID: s.ContainerID, Service: s.Name, Stderr: stderr}
	raw = strings.TrimRight(raw, "\r")
	if i := strings.IndexByte(raw, ' '); i > 0 {
		if t, err := time.Parse(time.RFC3339Nano, raw[:i]); err == nil {
			l.Time = t
			l.Text = raw[i+1:]
			return l
		}
	}
	l.Text = raw
	return l
}

func leftPad(s string, n int) string {
	for len(s) < n {
		s = "0" + s
	}
	return s
}
