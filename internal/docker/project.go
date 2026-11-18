// Package docker talks to the Docker daemon: it lists containers, groups them
// into compose projects, and streams events, logs and stats.
package docker

import (
	"fmt"
	"sort"
	"strings"

	"github.com/docker/docker/api/types/container"
)

// Compose labels. See the Reference section of PLAN.md.
const (
	LabelProject     = "com.docker.compose.project"
	LabelWorkingDir  = "com.docker.compose.project.working_dir"
	LabelConfigFiles = "com.docker.compose.project.config_files"
	LabelService     = "com.docker.compose.service"
	LabelNumber      = "com.docker.compose.container-number"
	LabelOneoff      = "com.docker.compose.oneoff"
)

// OtherProject holds containers that aren't part of a compose project.
const OtherProject = "Other"

// Project is one compose project and its containers.
type Project struct {
	Name        string
	WorkingDir  string
	ConfigFiles []string
	Services    []Service
	Running     int
	Total       int
}

// IsOther reports whether p is the catch-all group for non-compose containers.
func (p Project) IsOther() bool { return p.Name == OtherProject && p.WorkingDir == "" }

// Service is one container of a project. A service scaled to several replicas
// shows up once per container.
type Service struct {
	Name          string // compose service name (with "-N" for replicas past the first)
	Service       string // compose service name as in the compose file
	ContainerID   string
	ContainerName string
	Image         string
	State         string // running, exited, created, paused, restarting, dead
	Status        string // human status from the daemon, e.g. "Up 3 minutes"
	Ports         string
}

// Running reports whether the container is up.
func (s Service) Running() bool { return s.State == "running" || s.State == "restarting" }

// Group turns a flat container list into compose projects, sorted with
// running projects first and then by name. Containers without a project
// label go into a trailing "Other" group.
func Group(containers []container.Summary) []Project {
	byName := map[string]*Project{}
	var other *Project

	for _, c := range containers {
		name := c.Labels[LabelProject]
		var p *Project
		if name == "" {
			if other == nil {
				other = &Project{Name: OtherProject}
			}
			p = other
		} else {
			p = byName[name]
			if p == nil {
				p = &Project{Name: name}
				byName[name] = p
			}
			if p.WorkingDir == "" {
				p.WorkingDir = c.Labels[LabelWorkingDir]
			}
			if len(p.ConfigFiles) == 0 {
				p.ConfigFiles = splitList(c.Labels[LabelConfigFiles])
			}
		}
		s := toService(c)
		p.Services = append(p.Services, s)
		p.Total++
		if s.Running() {
			p.Running++
		}
	}

	out := make([]Project, 0, len(byName)+1)
	for _, p := range byName {
		sortServices(p.Services)
		out = append(out, *p)
	}
	Sort(out)
	if other != nil {
		sortServices(other.Services)
		out = append(out, *other)
	}
	return out
}

// Sort orders projects: running ones first, then by name.
func Sort(ps []Project) {
	sort.SliceStable(ps, func(i, j int) bool {
		ri, rj := ps[i].Running > 0, ps[j].Running > 0
		if ri != rj {
			return ri
		}
		return ps[i].Name < ps[j].Name
	})
}

func sortServices(ss []Service) {
	sort.SliceStable(ss, func(i, j int) bool {
		if ss[i].Name != ss[j].Name {
			return ss[i].Name < ss[j].Name
		}
		return ss[i].ContainerName < ss[j].ContainerName
	})
}

func toService(c container.Summary) Service {
	cname := ""
	if len(c.Names) > 0 {
		cname = strings.TrimPrefix(c.Names[0], "/")
	}
	svc := c.Labels[LabelService]
	name := svc
	if name == "" {
		name = cname
	} else {
		if n := c.Labels[LabelNumber]; n != "" && n != "1" {
			name = svc + "-" + n
		}
		if strings.EqualFold(c.Labels[LabelOneoff], "true") {
			name = svc + " (run)"
		}
	}
	return Service{
		Name:          name,
		Service:       svc,
		ContainerID:   c.ID,
		ContainerName: cname,
		Image:         c.Image,
		State:         c.State,
		Status:        c.Status,
		Ports:         formatPorts(c.Ports),
	}
}

func splitList(s string) []string {
	var out []string
	for _, f := range strings.Split(s, ",") {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// formatPorts renders published ports as "8000->8000", deduplicating the
// IPv4/IPv6 pair the daemon reports for each binding.
func formatPorts(ports []container.Port) string {
	seen := map[string]bool{}
	var out []string
	for _, p := range ports {
		if p.PublicPort == 0 {
			continue
		}
		s := fmt.Sprintf("%d->%d", p.PublicPort, p.PrivatePort)
		if p.Type != "" && p.Type != "tcp" {
			s += "/" + p.Type
		}
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return strings.Join(out, " ")
}
