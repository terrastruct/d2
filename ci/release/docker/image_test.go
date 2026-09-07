package docker_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDockerImageSmoke(t *testing.T) {
	for _, tc := range []struct {
		name        string
		legacy      bool
		wantSuccess bool
	}{
		{name: "current", wantSuccess: true},
		{name: "tala-unavailable"},
		{name: "tala-error"},
		{name: "tala-missing-output"},
		{name: "tala-invalid-svg"},
		{name: "tala-unavailable", legacy: true, wantSuccess: true},
		{name: "invalid-svg", legacy: true},
		{name: "invalid-png", legacy: true},
	} {
		name := tc.name
		if tc.legacy {
			name = "legacy/" + name
		}
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			bin := filepath.Join(directory, "bin")
			if err := os.Mkdir(bin, 0o755); err != nil {
				t.Fatal(err)
			}
			writeTestFile(t, filepath.Join(bin, "docker"), "#!/usr/bin/env bash\nset -euo pipefail\n"+fakeSmokeDocker)
			cmd := exec.Command("bash", "./image.sh", "smoke", "test-d2:smoke")
			// Exercise the unset default even if the developer's environment opts out.
			for _, entry := range os.Environ() {
				if !strings.HasPrefix(entry, "REQUIRE_TALA=") {
					cmd.Env = append(cmd.Env, entry)
				}
			}
			cmd.Env = append(cmd.Env,
				"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
				"ARCH=amd64", "VERSION=v1.2.3", "WORK_DIR="+directory,
				"TEST_SMOKE_CASE="+tc.name, "TEST_SMOKE_CALLS="+filepath.Join(directory, "calls"),
			)
			if tc.legacy {
				cmd.Env = append(cmd.Env, "REQUIRE_TALA=false")
			}
			output, err := cmd.CombinedOutput()
			if (err == nil) != tc.wantSuccess {
				t.Fatalf("success=%t, err=%v: %s", tc.wantSuccess, err, output)
			}
			calls, err := os.ReadFile(filepath.Join(directory, "calls"))
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Count(string(calls), "tala\n"); got != boolCount(!tc.legacy) {
				t.Fatalf("expected TALA render=%t, calls=%q: %s", !tc.legacy, calls, output)
			}
			if tc.wantSuccess {
				for _, call := range []string{"version\n", "svg\n", "png\n"} {
					if strings.Count(string(calls), call) != 1 {
						t.Fatalf("expected one %q call, got %q", call, calls)
					}
				}
			}
		})
	}
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}

const fakeSmokeDocker = `
[[ $1 == run ]] || { echo "unexpected Docker command: $*" >&2; exit 1; }
shift
mount=
while [[ $# -gt 0 ]]; do
  case "$1" in
    --rm) shift ;;
    --platform|-u) shift 2 ;;
    -v) mount=${2%:/home/debian/src}; shift 2 ;;
    test-d2:smoke) shift; break ;;
    *) echo "unexpected Docker argument: $1" >&2; exit 1 ;;
  esac
done
if [[ $# == 1 && $1 == --version ]]; then
  echo version >>"$TEST_SMOKE_CALLS"
  echo v1.2.3
  exit 0
fi
layout=default
args=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    --layout|-l) layout=$2; shift 2 ;;
    --layout=*) layout=${1#--layout=}; shift ;;
    *) args+=("$1"); shift ;;
  esac
done
[[ -n $mount && ${#args[@]} == 2 && -s "$mount/${args[0]}" ]] || exit 1
output="$mount/${args[1]}"
if [[ $layout == tala ]]; then
  echo tala >>"$TEST_SMOKE_CALLS"
  case "$TEST_SMOKE_CASE" in
    tala-unavailable) echo "unknown layout: tala" >&2; exit 1 ;;
    tala-error) echo "TALA layout failed" >&2; exit 2 ;;
    tala-missing-output) exit 0 ;;
    tala-invalid-svg) echo "not an SVG" >"$output"; exit 0 ;;
  esac
  printf '<svg></svg>\n' >"$output"
elif [[ $output == *.svg ]]; then
  echo svg >>"$TEST_SMOKE_CALLS"
  if [[ $TEST_SMOKE_CASE == invalid-svg ]]; then
    echo "not an SVG" >"$output"
  else
    printf '<svg></svg>\n' >"$output"
  fi
elif [[ $output == *.png ]]; then
  echo png >>"$TEST_SMOKE_CALLS"
  if [[ $TEST_SMOKE_CASE == invalid-png ]]; then
    echo "not a PNG" >"$output"
  else
    printf '\211PNG\r\n\032\n' >"$output"
  fi
else
  echo "unexpected render: $*" >&2; exit 1
fi
`
