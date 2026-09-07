package release_test

import (
	"embed"
	"os"
	"os/exec"
	"runtime"
	"testing"
)

// Include subprocess inputs in the test binary so script edits invalidate Go's test cache.
//
//go:embed prepare.py prepare_test.py upload-draft.py upload_draft_test.py checksum.sh
var scriptSources embed.FS

func TestMain(m *testing.M) {
	code := m.Run()
	runtime.KeepAlive(scriptSources) // Prevent the linker from discarding the cache inputs.
	os.Exit(code)
}

func TestDraftReleaseUploader(t *testing.T) {
	t.Parallel()
	cmd := exec.Command("python3", "./upload_draft_test.py")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("draft uploader tests failed: %v\n%s", err, output)
	}
}

func TestReleasePreparation(t *testing.T) {
	t.Parallel()
	cmd := exec.Command("python3", "./prepare_test.py")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("release preparation tests failed: %v\n%s", err, output)
	}
}
