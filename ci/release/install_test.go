package release

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGeneratedInstallerExplainsLegacyTALARequests(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the generated installer is a POSIX shell script")
	}
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	installer := filepath.Join(repositoryRoot, "install.sh")

	help, err := exec.Command("sh", installer, "--help").CombinedOutput()
	if err != nil {
		t.Fatalf("installer help: %v\n%s", err, help)
	}
	for _, want := range []string{"installs only D2", "--tala is no longer supported", "d2 layout", "https://github.com/terrastruct/TALA/releases"} {
		if !strings.Contains(string(help), want) {
			t.Fatalf("installer help is missing %q:\n%s", want, help)
		}
	}
}

func TestGeneratedInstallerRejectsLegacyTALABeforeInstallation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the generated installer is a POSIX shell script")
	}

	// Neither a guessed first bundled version nor the currently installed d2
	// establishes the capabilities of the requested release. Reject the legacy
	// installer operation before resolving latest, consulting Homebrew, or
	// changing an existing installation.
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"old_release", []string{"--method=standalone", "--version=v0.7.1", "--tala", "latest"}},
		{"latest_release", []string{"--method=standalone", "--version=latest", "--tala"}},
		{"default_release", []string{"--method=standalone", "--tala"}},
		{"homebrew", []string{"--method=homebrew", "--tala=v0.4.3"}},
		{"detect", []string{"--method=detect", "--tala"}},
		{"uninstall", []string{"--uninstall", "--tala"}},
	} {
		for _, dryRun := range []bool{false, true} {
			name := tc.name
			if dryRun {
				name += "_dry_run"
			}
			t.Run(name, func(t *testing.T) {
				temp := t.TempDir()
				bin := filepath.Join(temp, "bin")
				if err := os.Mkdir(bin, 0o755); err != nil {
					t.Fatal(err)
				}
				// A real install is safe to exercise here: any attempt to query
				// platforms, versions, networks, or package managers is recorded
				// and fails before it can perform external work.
				for _, program := range []string{"uname", "d2", "curl", "brew", "make"} {
					stub := "#!/bin/sh\nprintf '%s\\n' \"$0\" >> \"$INSTALLER_TEST_COMMAND_LOG\"\nexit 71\n"
					if err := os.WriteFile(filepath.Join(bin, program), []byte(stub), 0o755); err != nil {
						t.Fatal(err)
					}
				}
				prefix := filepath.Join(temp, "prefix")
				cache := filepath.Join(temp, "cache")
				commandLog := filepath.Join(temp, "commands")
				args := []string{filepath.Join("..", "..", "install.sh"), "--prefix=" + prefix}
				args = append(args, tc.args...)
				if dryRun {
					args = append(args, "--dry-run")
				}
				command := exec.Command("sh", args...)
				command.Env = append(os.Environ(),
					"PATH="+bin+":/usr/bin:/bin",
					"XDG_CACHE_HOME="+cache,
					"INSTALLER_TEST_COMMAND_LOG="+commandLog,
				)
				output, err := command.CombinedOutput()
				if err == nil {
					t.Fatalf("legacy TALA request succeeded:\n%s", output)
				}
				for _, want := range []string{"--tala is no longer supported", "installs only D2", "d2 layout", "https://github.com/terrastruct/TALA/releases"} {
					if !strings.Contains(string(output), want) {
						t.Fatalf("legacy TALA error is missing %q:\n%s", want, output)
					}
				}
				for _, path := range []string{commandLog, prefix, cache} {
					if _, err := os.Stat(path); !os.IsNotExist(err) {
						t.Fatalf("installer touched %s before rejecting --tala: %v", path, err)
					}
				}
			})
		}
	}
}

func TestGeneratedInstallerWithoutTALAStillPlansPinnedD2(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the generated installer is a POSIX shell script")
	}

	temp := t.TempDir()
	command := exec.Command("sh", filepath.Join("..", "..", "install.sh"),
		"--dry-run", "--method=standalone", "--version=v0.7.1", "--prefix="+filepath.Join(temp, "prefix"))
	command.Env = append(os.Environ(),
		"PATH=/usr/bin:/bin",
		"XDG_CACHE_HOME="+filepath.Join(temp, "cache"),
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("pinned D2 dry run: %v\n%s", err, output)
	}
	text := string(output)
	if !strings.Contains(text, "installing standalone release d2-v0.7.1-") {
		t.Fatalf("pinned D2 installation missing:\n%s", output)
	}
	for _, stale := range []string{"d2plugin-tala", "github.com/terrastruct", "terrastruct/tap/tala", "installing tala-", "--tala"} {
		if strings.Contains(text, stale) {
			t.Fatalf("plain D2 dry run contains %q:\n%s", stale, output)
		}
	}
}
