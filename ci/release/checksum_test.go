package release_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseAssetDigestParser(t *testing.T) {
	t.Parallel()
	metadata := `{
  "name": "v1.2.3",
  "assets": [
    {
      "name": "other.tar.gz",
      "digest": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
    },
    {
      "name": "d2-v1.2.3-linux-amd64.tar.gz",
      "uploader": {
        "login": "example"
      },
      "digest": "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
    }
  ]
}`
	got := runChecksumFunction(t, metadata, "release_asset_digest_from_json", "d2-v1.2.3-linux-amd64.tar.gz")
	want := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if got != want {
		t.Fatalf("digest = %q, want %q", got, want)
	}
}

func TestReleaseAssetDigestParserRejectsMissingDigest(t *testing.T) {
	t.Parallel()
	metadata := `{
  "assets": [
    {
      "name": "d2-v1.2.3-linux-amd64.tar.gz",
      "digest": null
    }
  ]
}`
	if got := runChecksumFunction(t, metadata, "release_asset_digest_from_json", "d2-v1.2.3-linux-amd64.tar.gz"); got != "" {
		t.Fatalf("digest = %q, want empty", got)
	}
}

func TestVerifySHA256(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "asset")
	if err := os.WriteFile(file, []byte("d2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	const digest = "c66169a0e4913133cbebf50c3f71dabef7540b9a636cde9c80be6e39f7113fa5"
	command := `. "$1"; verify_sha256 "$2" "$3"`
	cmd := exec.Command("sh", "-c", command, "sh", "./checksum.sh", file, digest)
	cmd.Dir = "."
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("verify_sha256 failed: %v\n%s", err, output)
	}
	cmd = exec.Command("sh", "-c", command, "sh", "./checksum.sh", file, strings.Repeat("0", 64))
	cmd.Dir = "."
	if err := cmd.Run(); err == nil {
		t.Fatal("verify_sha256 accepted an incorrect digest")
	}
}

func runChecksumFunction(t *testing.T, input, function string, args ...string) string {
	t.Helper()
	command := `. "$1"; shift; function=$1; shift; "$function" "$@"`
	cmdArgs := []string{"-c", command, "sh", "./checksum.sh", function}
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.Command("sh", cmdArgs...)
	cmd.Dir = "."
	cmd.Stdin = strings.NewReader(input)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s failed: %v\n%s", function, err, output)
	}
	return strings.TrimSpace(string(output))
}
