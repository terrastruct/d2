package release_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var releaseTargets = []string{
	"linux-amd64",
	"linux-arm64",
	"macos-amd64",
	"macos-arm64",
	"windows-amd64",
	"windows-arm64",
}

func TestVerifyCIReleaseAssetDirectory(t *testing.T) {
	t.Parallel()
	directory := createReleaseAssets(t)
	verifyReleaseAssetDirectory(t, directory, true)
}

func TestVerifyCIReleaseAssetDirectoryRejectsUnexpectedFile(t *testing.T) {
	t.Parallel()
	directory := createReleaseAssets(t)
	writeEmptyFile(t, filepath.Join(directory, ".DS_Store"))
	verifyReleaseAssetDirectoryFails(t, directory, true, "unexpected file")
}

func TestVerifyCIReleaseAssetDirectoryRejectsSymlink(t *testing.T) {
	t.Parallel()
	directory := createReleaseAssets(t)
	target := filepath.Join(t.TempDir(), "local-file")
	writeEmptyFile(t, target)
	asset := filepath.Join(directory, "d2-v1.2.3-linux-amd64.tar.gz")
	if err := os.Remove(asset); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, asset); err != nil {
		t.Fatal(err)
	}
	verifyReleaseAssetDirectoryFails(t, directory, true, "must not be a symlink")
}

func TestVerifyCIReleaseAssetDirectoryRejectsSymlinkedDirectory(t *testing.T) {
	t.Parallel()
	target := createReleaseAssets(t)
	directory := filepath.Join(t.TempDir(), "release-assets")
	if err := os.Symlink(target, directory); err != nil {
		t.Fatal(err)
	}
	verifyReleaseAssetDirectoryFails(t, directory, true, "directory must not be a symlink")
}

func TestVerifyCIReleaseAssetDirectoryRejectsMissingAsset(t *testing.T) {
	t.Parallel()
	directory := createReleaseAssets(t)
	if err := os.Remove(filepath.Join(directory, "d2-v1.2.3-windows-arm64.tar.gz")); err != nil {
		t.Fatal(err)
	}
	verifyReleaseAssetDirectoryFails(t, directory, true, "is missing")
}

func TestVerifyCIReleaseAssetDirectoryAllowsExpectedSubsetDuringReplacement(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	writeEmptyFile(t, filepath.Join(directory, "d2-v1.2.3-linux-amd64.tar.gz"))
	verifyReleaseAssetDirectory(t, directory, false)
	writeEmptyFile(t, filepath.Join(directory, "notes.txt"))
	verifyReleaseAssetDirectoryFails(t, directory, false, "unexpected file")
}

func TestReleaseRejectsSkipBuild(t *testing.T) {
	t.Parallel()
	cmd := exec.Command("sh", "./release.sh", "--skip-build", "--version=v1.2.3")
	cmd.Dir = "."
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("release.sh --skip-build unexpectedly succeeded")
	}
	if !strings.Contains(string(output), "no longer supports --skip-build") {
		t.Fatalf("unexpected failure:\n%s", output)
	}
}

func createReleaseAssets(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	for _, target := range releaseTargets {
		writeEmptyFile(t, filepath.Join(directory, "d2-v1.2.3-"+target+".tar.gz"))
	}
	writeEmptyFile(t, filepath.Join(directory, "SHA256SUMS"))
	return directory
}

func writeEmptyFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

func verifyReleaseAssetDirectory(t *testing.T, directory string, requireAll bool) {
	t.Helper()
	if output, err := releaseAssetCommand(directory, requireAll).CombinedOutput(); err != nil {
		t.Fatalf("verify_ci_release_asset_directory failed: %v\n%s", err, output)
	}
}

func verifyReleaseAssetDirectoryFails(t *testing.T, directory string, requireAll bool, message string) {
	t.Helper()
	output, err := releaseAssetCommand(directory, requireAll).CombinedOutput()
	if err == nil {
		t.Fatal("verify_ci_release_asset_directory unexpectedly succeeded")
	}
	if !strings.Contains(string(output), message) {
		t.Fatalf("failure does not contain %q:\n%s", message, output)
	}
}

func releaseAssetCommand(directory string, requireAll bool) *exec.Cmd {
	requireAllArg := "1"
	if !requireAll {
		requireAllArg = "0"
	}
	command := `. "$1"; verify_ci_release_asset_directory "$2" v1.2.3 "$3"`
	cmd := exec.Command("sh", "-c", command, "sh", "./assets.sh", directory, requireAllArg)
	cmd.Dir = "."
	return cmd
}
