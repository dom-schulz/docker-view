package docker

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

// Client wraps the Docker SDK client.
type Client struct {
	api  *client.Client
	host string
}

// New connects using DOCKER_HOST when set, falling back to
// /var/run/docker.sock (and Docker Desktop's per-user socket on the Mac).
func New() (*Client, error) {
	opts := []client.Opt{client.FromEnv, client.WithAPIVersionNegotiation()}
	host := os.Getenv("DOCKER_HOST")
	if host == "" {
		host = "unix:///var/run/docker.sock"
		if _, err := os.Stat("/var/run/docker.sock"); err != nil {
			if home, herr := os.UserHomeDir(); herr == nil {
				alt := filepath.Join(home, ".docker", "run", "docker.sock")
				if _, aerr := os.Stat(alt); aerr == nil {
					host = "unix://" + alt
				}
			}
		}
		opts = append(opts, client.WithHost(host))
	}
	api, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, err
	}
	return &Client{api: api, host: host}, nil
}

// Host is the daemon address in use, for error messages.
func (c *Client) Host() string { return c.host }

// Ping checks that the daemon answers.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.api.Ping(ctx)
	return err
}

// Projects lists every container, running or not, grouped by project.
func (c *Client) Projects(ctx context.Context) ([]Project, error) {
	cs, err := c.api.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, err
	}
	return Group(cs), nil
}

// Close releases the client.
func (c *Client) Close() error { return c.api.Close() }

// Watch sends on the returned channel whenever a container starts, stops,
// is created or destroyed. If the event stream drops it reconnects with
// backoff and signals once more so the caller re-lists anything it missed.
// The channel has a buffer of one, so bursts collapse into a single signal.
func (c *Client) Watch(ctx context.Context) <-chan struct{} {
	out := make(chan struct{}, 1)
	notify := func() {
		select {
		case out <- struct{}{}:
		default:
		}
	}
	f := filters.NewArgs(filters.Arg("type", string(events.ContainerEventType)))
	for _, a := range []string{"start", "stop", "die", "create", "destroy", "pause", "unpause", "rename"} {
		f.Add("event", a)
	}

	go func() {
		backoff := time.Second
		for {
			msgs, errs := c.api.Events(ctx, events.ListOptions{Filters: f})
		stream:
			for {
				select {
				case <-ctx.Done():
					return
				case <-msgs:
					backoff = time.Second
					notify()
				case <-errs:
					break stream
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			notify()
		}
	}()
	return out
}
