package compose

import (
	"reflect"
	"testing"
)

func TestArgs(t *testing.T) {
	b := Builder{Bin: "docker", Prefix: []string{"compose"}, Exists: func(string) bool { return true }}
	ref := Ref{Project: "api-feature-x", WorkingDir: "/w/api", ConfigFiles: []string{"/w/api/compose.yaml", "/w/api/compose.dev.yaml"}}
	got := b.Args(ref, "up", "-d", "db")
	want := []string{"compose", "-p", "api-feature-x", "--project-directory", "/w/api",
		"-f", "/w/api/compose.yaml", "-f", "/w/api/compose.dev.yaml", "up", "-d", "db"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args =\n %v\nwant\n %v", got, want)
	}
	if got := b.Command(ref, "down")[0]; got != "docker" {
		t.Errorf("command starts with %q", got)
	}
}

func TestArgsSkipsMissingPaths(t *testing.T) {
	b := Builder{Bin: "docker", Prefix: []string{"compose"}, Exists: func(p string) bool { return p == "/w/api/compose.yaml" }}
	ref := Ref{Project: "gone", WorkingDir: "/w/gone", ConfigFiles: []string{"/w/gone/compose.yaml", "/w/api/compose.yaml"}}
	got := b.Args(ref, "down")
	want := []string{"compose", "-p", "gone", "-f", "/w/api/compose.yaml", "down"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %v, want %v", got, want)
	}
}

func TestStandaloneBinary(t *testing.T) {
	b := Builder{Bin: "docker-compose", Exists: func(string) bool { return false }}
	got := b.Command(Ref{Project: "p"}, "stop")
	if want := []string{"docker-compose", "-p", "p", "stop"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("command = %v, want %v", got, want)
	}
}

func TestQuote(t *testing.T) {
	got := Quote([]string{"docker", "compose", "--project-directory", "/my dir", "it's"})
	if want := `docker compose --project-directory '/my dir' 'it'\''s'`; got != want {
		t.Errorf("quote = %s, want %s", got, want)
	}
}
