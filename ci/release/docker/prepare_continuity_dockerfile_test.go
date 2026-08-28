package docker_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareV082DockerfileMakesPlaywrightNonInteractive(t *testing.T) {
	t.Parallel()

	input := "FROM ubuntu:24.04\nUSER debian:debian\nRUN d2 init-playwright\nENTRYPOINT [\"d2\"]\n"
	output := runPrepareDockerfile(t, "v0.8.2", input, true)
	want := strings.Replace(input, "RUN d2 init-playwright", "RUN CI=1 d2 init-playwright", 1)
	if output != want {
		t.Fatalf("prepared Dockerfile differs:\nwant:\n%s\ngot:\n%s", want, output)
	}
}

func TestReleaseDockerfileInstallsOnePlaywrightBrowser(t *testing.T) {
	t.Parallel()

	dockerfile, err := os.ReadFile("Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	source := string(dockerfile)
	if strings.Contains(source, "install --with-deps chromium") {
		t.Fatal("release Dockerfile downloads a root-owned Playwright browser")
	}
	if got := strings.Count(source, "install-deps chromium"); got != 1 {
		t.Fatalf("release Dockerfile has %d Playwright dependency installs, want 1", got)
	}
	if got := strings.Count(source, "RUN CI=1 d2 init-playwright"); got != 1 {
		t.Fatalf("release Dockerfile has %d runtime Playwright installs, want 1", got)
	}
}

func TestPrepareV082DockerfileRejectsUnexpectedSource(t *testing.T) {
	t.Parallel()

	runPrepareDockerfile(t, "v0.8.2", "FROM ubuntu:24.04\n", false)
}

func TestPrepareOtherDockerfileCopiesExactly(t *testing.T) {
	t.Parallel()

	input := "FROM scratch\nCOPY d2 /d2\n"
	if got := runPrepareDockerfile(t, "v1.2.3", input, true); got != input {
		t.Fatalf("prepared Dockerfile differs:\nwant:\n%s\ngot:\n%s", input, got)
	}
}

func runPrepareDockerfile(t *testing.T, version, input string, wantSuccess bool) string {
	t.Helper()

	tempDir := t.TempDir()
	source := filepath.Join(tempDir, "Dockerfile.in")
	output := filepath.Join(tempDir, "Dockerfile.out")
	if err := os.WriteFile(source, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("bash", "./prepare-continuity-dockerfile.sh", version, source, output)
	combined, err := cmd.CombinedOutput()
	if wantSuccess && err != nil {
		t.Fatalf("prepare Dockerfile failed: %v\n%s", err, combined)
	}
	if !wantSuccess {
		if err == nil {
			t.Fatalf("prepare Dockerfile unexpectedly succeeded:\n%s", combined)
		}
		if !bytes.Contains(combined, []byte("expected exactly one interactive d2 init-playwright instruction")) {
			t.Fatalf("unexpected failure:\n%s", combined)
		}
		return ""
	}

	prepared, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	return string(prepared)
}
