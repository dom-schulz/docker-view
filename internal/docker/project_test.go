package docker

import (
	"reflect"
	"testing"

	"github.com/docker/docker/api/types/container"
)

func ctr(name, project, service, state string, extra ...string) container.Summary {
	labels := map[string]string{}
	if project != "" {
		labels[LabelProject] = project
		labels[LabelService] = service
		labels[LabelWorkingDir] = "/src/" + project
		labels[LabelConfigFiles] = "/src/" + project + "/compose.yaml,/src/" + project + "/compose.override.yaml"
	}
	for i := 0; i+1 < len(extra); i += 2 {
		labels[extra[i]] = extra[i+1]
	}
	return container.Summary{ID: name + "-id", Names: []string{"/" + name}, Labels: labels, State: state}
}

func names(ps []Project) []string {
	var out []string
	for _, p := range ps {
		out = append(out, p.Name)
	}
	return out
}

func TestGroupAndSort(t *testing.T) {
	ps := Group([]container.Summary{
		ctr("old-spike-web-1", "old-spike", "web", "exited"),
		ctr("api-main-db-1", "api-main", "db", "exited"),
		ctr("random", "", "", "running"),
		ctr("demo-webapp-frontend-1", "demo-webapp", "frontend", "running"),
		ctr("demo-webapp-backend-1", "demo-webapp", "backend", "running"),
		ctr("api-feature-x-db-1", "api-feature-x", "db", "running"),
		ctr("api-feature-x-api-1", "api-feature-x", "api", "exited"),
	})

	want := []string{"api-feature-x", "demo-webapp", "api-main", "old-spike", OtherProject}
	if got := names(ps); !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}

	fx := ps[0]
	if fx.Running != 1 || fx.Total != 2 {
		t.Errorf("api-feature-x running/total = %d/%d, want 1/2", fx.Running, fx.Total)
	}
	if fx.WorkingDir != "/src/api-feature-x" {
		t.Errorf("working dir = %q", fx.WorkingDir)
	}
	if want := []string{"/src/api-feature-x/compose.yaml", "/src/api-feature-x/compose.override.yaml"}; !reflect.DeepEqual(fx.ConfigFiles, want) {
		t.Errorf("config files = %v, want %v", fx.ConfigFiles, want)
	}
	if got := []string{fx.Services[0].Name, fx.Services[1].Name}; !reflect.DeepEqual(got, []string{"api", "db"}) {
		t.Errorf("services = %v, want sorted [api db]", got)
	}

	other := ps[len(ps)-1]
	if !other.IsOther() || other.Services[0].Name != "random" || other.Running != 1 {
		t.Errorf("other group = %+v", other)
	}
}

func TestOtherLastEvenWhenOnlyItRuns(t *testing.T) {
	ps := Group([]container.Summary{
		ctr("loose", "", "", "running"),
		ctr("zeta-a-1", "zeta", "a", "exited"),
	})
	if got := names(ps); !reflect.DeepEqual(got, []string{"zeta", OtherProject}) {
		t.Fatalf("order = %v", got)
	}
}

func TestReplicaAndOneoffNames(t *testing.T) {
	ps := Group([]container.Summary{
		ctr("p-web-1", "p", "web", "running", LabelNumber, "1"),
		ctr("p-web-2", "p", "web", "running", LabelNumber, "2"),
		ctr("p-web-run-abc", "p", "web", "running", LabelOneoff, "True"),
	})
	var got []string
	for _, s := range ps[0].Services {
		got = append(got, s.Name)
		if s.Service != "web" {
			t.Errorf("compose service = %q, want web", s.Service)
		}
	}
	if want := []string{"web", "web (run)", "web-2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("names = %v, want %v", got, want)
	}
}

func TestFormatPorts(t *testing.T) {
	got := formatPorts([]container.Port{
		{IP: "0.0.0.0", PrivatePort: 8000, PublicPort: 8000, Type: "tcp"},
		{IP: "::", PrivatePort: 8000, PublicPort: 8000, Type: "tcp"},
		{PrivatePort: 9000, Type: "tcp"},
		{IP: "0.0.0.0", PrivatePort: 53, PublicPort: 5353, Type: "udp"},
	})
	if want := "5353->53/udp 8000->8000"; got != want {
		t.Errorf("ports = %q, want %q", got, want)
	}
}

func TestParseLogLine(t *testing.T) {
	s := Service{Name: "api", ContainerID: "c1"}
	l := ParseLogLine("2026-11-17T10:00:00.123456789Z hello world\r", s, true)
	if l.Text != "hello world" || l.Time.Nanosecond() != 123456789 || !l.Stderr || l.Service != "api" {
		t.Errorf("parsed = %+v", l)
	}
	if l := ParseLogLine("no timestamp here", s, false); l.Text != "no timestamp here" || !l.Time.IsZero() {
		t.Errorf("parsed = %+v", l)
	}
}

func TestComputeStats(t *testing.T) {
	var v container.StatsResponse
	v.CPUStats.CPUUsage.TotalUsage = 2_000
	v.PreCPUStats.CPUUsage.TotalUsage = 1_000
	v.CPUStats.SystemUsage = 20_000
	v.PreCPUStats.SystemUsage = 10_000
	v.CPUStats.OnlineCPUs = 4
	v.MemoryStats.Usage = 300
	v.MemoryStats.Limit = 1000
	v.MemoryStats.Stats = map[string]uint64{"inactive_file": 100}
	s := ComputeStats("x", v)
	if s.CPUPercent != 40 {
		t.Errorf("cpu = %v, want 40", s.CPUPercent)
	}
	if s.MemUsage != 200 || s.MemPercent() != 20 {
		t.Errorf("mem = %d (%v%%), want 200 (20%%)", s.MemUsage, s.MemPercent())
	}
}
