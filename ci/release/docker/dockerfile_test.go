package docker_test

import (
	"os"
	"strings"
	"testing"
)

func TestReleaseDockerfileKeepsRuntimeDependenciesMinimal(t *testing.T) {
	t.Parallel()

	dockerfile, err := os.ReadFile("Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	source := string(dockerfile)
	const packages = "apt-get install -y --no-install-recommends adduser ca-certificates curl dumb-init sudo"
	if !strings.Contains(source, packages) {
		t.Fatal("release Dockerfile does not install the expected runtime packages")
	}
	if got := strings.Count(source, "apt-get install"); got != 1 {
		t.Fatalf("release Dockerfile has %d package installation steps, want 1", got)
	}

	const runtimeUser = "\nUSER debian:debian\n"
	runtimeStart := strings.Index(source, runtimeUser)
	if runtimeStart < 0 {
		t.Fatal("release Dockerfile does not switch to the runtime user")
	}
	runtimeSource := source[runtimeStart+len(runtimeUser):]
	if strings.HasPrefix(runtimeSource, "RUN ") || strings.Contains(runtimeSource, "\nRUN ") {
		t.Fatal("release Dockerfile performs setup after switching to the runtime user")
	}
}
