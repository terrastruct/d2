package docker_test

import (
	"crypto/sha256"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Include subprocess inputs in the test binary so script edits invalidate Go's test cache.
//
//go:embed common.sh release.sh
var scriptSources embed.FS

func TestMain(m *testing.M) {
	code := m.Run()
	runtime.KeepAlive(scriptSources) // Prevent the linker from discarding the cache inputs.
	os.Exit(code)
}

func TestDockerReleaseVersion(t *testing.T) {
	for _, version := range []string{"v1.2.3", "v0.0.0-ci", "v1.2.3-rc.1", "1.2.3", "v01.2.3", "v1.2.3\nother", "v1.2.3+build"} {
		t.Run(version, func(t *testing.T) {
			cmd := exec.Command("bash", "-c", `. ./common.sh; validate_version "$1"`, "bash", version)
			output, err := cmd.CombinedOutput()
			valid := version == "v1.2.3" || version == "v0.0.0-ci" || version == "v1.2.3-rc.1"
			if (err == nil) != valid {
				t.Fatalf("valid=%t, err=%v: %s", valid, err, output)
			}
		})
	}
}

func TestDockerReleaseMetadata(t *testing.T) {
	for _, scenario := range []string{"valid", "draft", "unpublished", "duplicate", "empty", "digest", "tag"} {
		t.Run(scenario, func(t *testing.T) {
			metadata := releaseMetadata()
			assets := metadata["assets"].([]map[string]any)
			switch scenario {
			case "draft":
				metadata["draft"] = true
			case "unpublished":
				metadata["published_at"] = nil
			case "duplicate":
				metadata["assets"] = append(assets, assets[0])
			case "empty":
				assets[0]["size"] = 0
			case "digest":
				assets[0]["digest"] = "sha256:bad"
			case "tag":
				metadata["tag_name"] = "v9.9.9"
			}
			cmd := exec.Command("bash", "-c", `. ./common.sh; release_metadata v1.2.3`)
			cmd.Stdin = strings.NewReader(jsonString(t, metadata))
			output, err := cmd.CombinedOutput()
			if (err == nil) != (scenario == "valid") {
				t.Fatalf("err=%v: %s", err, output)
			}
		})
	}
}

func TestDockerCandidateProvenance(t *testing.T) {
	for _, scenario := range []string{"valid", "missing-arch", "duplicate-arch", "missing-provenance", "wrong-reference"} {
		t.Run(scenario, func(t *testing.T) {
			candidate := candidateManifest()
			manifests := candidate["manifests"].([]map[string]any)
			switch scenario {
			case "missing-arch":
				candidate["manifests"] = manifests[1:]
			case "duplicate-arch":
				manifests[1]["platform"] = manifests[0]["platform"]
			case "missing-provenance":
				candidate["manifests"] = manifests[:3]
			case "wrong-reference":
				manifests[3]["annotations"].(map[string]any)["vnd.docker.reference.digest"] = digest("9")
			}
			path := filepath.Join(t.TempDir(), "candidate.json")
			writeTestFile(t, path, jsonString(t, candidate))
			cmd := exec.Command("bash", "-c", `. ./common.sh; validate_candidate "$1"`, "bash", path)
			output, err := cmd.CombinedOutput()
			if (err == nil) != (scenario == "valid") {
				t.Fatalf("err=%v: %s", err, output)
			}
		})
	}
}

func TestDockerPublicationGuards(t *testing.T) {
	for _, scenario := range []string{"publish", "matching-retry", "existing-conflict", "release-changed", "tag-moved", "asset-changed", "not-ancestor", "registry-error", "postpush-mismatch", "preflight-existing", "immutability-disabled", "latest-disabled", "latest-enabled"} {
		t.Run(scenario, func(t *testing.T) {
			directory, env := publicationFixture(t)
			command := "publish-version"
			wantSuccess, wantWrites := false, false
			switch scenario {
			case "publish":
				wantSuccess, wantWrites = true, true
			case "matching-retry":
				env = append(env, "TEST_TAG_STATUS=200")
				wantSuccess = true
			case "existing-conflict":
				env = append(env, "TEST_TAG_STATUS=200", "TEST_TAG_DIGEST="+digest("9"))
			case "release-changed", "asset-changed":
				metadata := releaseMetadata()
				if scenario == "release-changed" {
					metadata["id"] = 999
				} else {
					metadata["assets"].([]map[string]any)[0]["id"] = 999
				}
				writeTestFile(t, filepath.Join(directory, "release.json"), jsonString(t, metadata))
			case "tag-moved":
				env = append(env, "TEST_COMMIT="+strings.Repeat("9", 40))
			case "not-ancestor":
				env = append(env, "TEST_COMPARE_STATUS=diverged")
			case "registry-error":
				env = append(env, "TEST_TAG_STATUS=503")
			case "postpush-mismatch":
				env = append(env, "TEST_POSTPUSH_MISMATCH=1")
				wantWrites = true
			case "preflight-existing":
				command = "preflight"
				env = append(env, "TEST_TAG_STATUS=200")
			case "immutability-disabled":
				command = "preflight"
				env = append(env, "TEST_IMMUTABLE=false")
			case "latest-disabled", "latest-enabled":
				command = "publish-latest"
				if scenario == "latest-enabled" {
					env = append(env, "PUBLISH_LATEST=true")
					wantSuccess, wantWrites = true, true
				}
			}
			cmd := exec.Command("bash", "./release.sh", command, filepath.Join(directory, "digests"))
			cmd.Env = append(os.Environ(), env...)
			output, err := cmd.CombinedOutput()
			if (err == nil) != wantSuccess {
				t.Fatalf("success=%t, err=%v: %s", wantSuccess, err, output)
			}
			writes, _ := os.ReadFile(filepath.Join(directory, "writes"))
			if (len(writes) != 0) != wantWrites {
				t.Fatalf("want writes=%t, got %q: %s", wantWrites, writes, output)
			}
			if (scenario == "publish" || scenario == "latest-enabled") && strings.Count(string(writes), "\n") != 2 {
				t.Fatalf("expected one publication per repository, got %q", writes)
			}
		})
	}
}

func TestDockerLatestRejectsPrerelease(t *testing.T) {
	for _, scenario := range []struct {
		version    string
		prerelease bool
	}{
		{"v1.2.3-rc.1", false},
		{"v1.2.3", true},
	} {
		cmd := exec.Command("bash", "-c", `. ./common.sh; require_stable`)
		cmd.Env = append(os.Environ(), "VERSION="+scenario.version, fmt.Sprintf(`RELEASE={"prerelease":%t}`, scenario.prerelease))
		if output, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(output), "cannot be used for a prerelease") {
			t.Fatalf("err=%v: %s", err, output)
		}
	}
}

func digest(character string) string { return "sha256:" + strings.Repeat(character, 64) }

func releaseMetadata() map[string]any {
	assets := []map[string]any{}
	for i, arch := range []string{"amd64", "arm64"} {
		assets = append(assets, map[string]any{
			"name": "d2-v1.2.3-linux-" + arch + ".tar.gz", "state": "uploaded", "size": 123,
			"id": i + 100, "digest": digest(fmt.Sprint(i + 1)),
		})
	}
	return map[string]any{"id": 42, "tag_name": "v1.2.3", "draft": false, "published_at": "2026-01-01", "prerelease": false, "assets": assets}
}

func candidateManifest() map[string]any {
	manifests := []map[string]any{}
	for i, arch := range []string{"amd64", "arm64"} {
		manifests = append(manifests, map[string]any{"digest": digest(fmt.Sprint(i + 1)), "platform": map[string]any{"os": "linux", "architecture": arch}})
	}
	for i := 0; i < 2; i++ {
		manifests = append(manifests, map[string]any{
			"digest": digest(fmt.Sprint(i + 3)), "platform": map[string]any{"os": "unknown", "architecture": "unknown"},
			"annotations": map[string]any{"vnd.docker.reference.type": "attestation-manifest", "vnd.docker.reference.digest": digest(fmt.Sprint(i + 1))},
		})
	}
	return map[string]any{"schemaVersion": 2, "manifests": manifests}
}

func publicationFixture(t *testing.T) (string, []string) {
	t.Helper()
	directory := t.TempDir()
	for _, subdir := range []string{"bin", "digests"} {
		if err := os.Mkdir(filepath.Join(directory, subdir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	candidate := jsonString(t, candidateManifest())
	hash := sha256.Sum256([]byte(candidate))
	manifestDigest := fmt.Sprintf("sha256:%x", hash)
	writeTestFile(t, filepath.Join(directory, "candidate.json"), candidate+"\n")
	writeTestFile(t, filepath.Join(directory, "release.json"), jsonString(t, releaseMetadata()))
	writeTestFile(t, filepath.Join(directory, "digests", "amd64.txt"), digest("a"))
	writeTestFile(t, filepath.Join(directory, "digests", "arm64.txt"), digest("b"))
	snapshot := map[string]any{
		"version": "v1.2.3", "id": 42, "prerelease": false, "commit": strings.Repeat("a", 40),
		"assets": map[string]any{"amd64": map[string]any{"id": 100, "digest": digest("1")}, "arm64": map[string]any{"id": 101, "digest": digest("2")}},
	}
	for name, source := range map[string]string{"gh": fakeGitHub, "curl": fakeRegistry, "docker": fakeDocker} {
		writeTestFile(t, filepath.Join(directory, "bin", name), "#!/usr/bin/env bash\nset -euo pipefail\n"+source)
	}
	return directory, []string{
		"PATH=" + filepath.Join(directory, "bin") + string(os.PathListSeparator) + os.Getenv("PATH"),
		"TEST_DIR=" + directory, "WORK_DIR=" + directory, "GITHUB_OUTPUT=" + filepath.Join(directory, "output"),
		"GITHUB_STEP_SUMMARY=" + filepath.Join(directory, "summary"), "GITHUB_REPOSITORY=d2lang/d2", "GITHUB_SHA=" + strings.Repeat("b", 40),
		"RELEASE=" + jsonString(t, snapshot), "PRIMARY_DOCKER_IMAGE=d2lang/d2", "LEGACY_DOCKER_IMAGE=terrastruct/d2", "TEST_MANIFEST_DIGEST=" + manifestDigest,
		"VERSION_DIGEST=" + manifestDigest, "PUBLISH_LATEST=false",
	}
}

func jsonString(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

const fakeGitHub = `
case "$2" in
  */releases/tags/*) cat "$TEST_DIR/release.json" ;;
  */git/ref/tags/*) printf '{"object":{"type":"commit","sha":"%s"}}' "${TEST_COMMIT:-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa}" ;;
  */compare/*) echo "${TEST_COMPARE_STATUS:-ahead}" ;;
  *) echo "unexpected GitHub request: $*" >&2; exit 1 ;;
esac
`

const fakeRegistry = `
headers= url=
while [[ $# -gt 0 ]]; do
  case "$1" in
    -D) headers=$2; shift ;;
    https://*) url=$1 ;;
  esac
  shift
done
case "$url" in
  https://auth.docker.io/*) echo '{"token":"offline-test"}'; exit 0 ;;
  https://hub.docker.com/*)
    printf '{"immutable_tags_settings":{"enabled":%s,"rules":["^v[0-9]+\\\\.[0-9]+\\\\.[0-9]+(-[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?$"]}}' "${TEST_IMMUTABLE:-true}"
    exit 0 ;;
  https://registry-1.docker.io/*) ;;
  *) echo "unexpected registry request: $url" >&2; exit 1 ;;
esac
image=${url#*/v2/}
image=${image%/manifests/*}
status=${TEST_TAG_STATUS:-404}
digest=${TEST_TAG_DIGEST:-$TEST_MANIFEST_DIGEST}
if [[ -f "$TEST_DIR/${image//\//-}.published" ]]; then
  status=200
  if [[ ${TEST_POSTPUSH_MISMATCH:-0} == 1 ]]; then digest=sha256:9999999999999999999999999999999999999999999999999999999999999999; fi
fi
printf 'Docker-Content-Digest: %s\r\n' "$digest" >"$headers"
printf '%s' "$status"
`

const fakeDocker = `
if [[ "$*" == *'--dry-run'* || "$*" == *'inspect --raw'* ]]; then
  cat "$TEST_DIR/candidate.json"
elif [[ "$*" == *'imagetools create'* ]]; then
  printf '%s\n' "$*" >>"$TEST_DIR/writes"
  tag= metadata=
  while [[ $# -gt 0 ]]; do
    case "$1" in --tag) tag=$2; shift ;; --metadata-file) metadata=$2; shift ;; esac
    shift
  done
  image=${tag%:*}
  touch "$TEST_DIR/${image//\//-}.published"
  if [[ -n $metadata ]]; then
    printf '{"containerimage.descriptor":{"digest":"%s"}}' "$TEST_MANIFEST_DIGEST" >"$metadata"
  fi
elif [[ "$*" != *'imagetools inspect'* ]]; then
  echo "unexpected Docker command: $*" >&2; exit 1
fi
`
