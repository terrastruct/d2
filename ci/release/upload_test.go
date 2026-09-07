package release_test

import (
	"os/exec"
	"testing"
)

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
