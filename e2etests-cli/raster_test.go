package e2etests_cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/d2lang/util-go/xos"

	"github.com/d2lang/d2/internal/testutil"
	"github.com/d2lang/d2/internal/testutil/imagediff"
)

func TestPNGFixtures(t *testing.T) {
	t.Parallel()

	for _, fixture := range []string{"geometry", "markdown", "local-icon", "unlabelled-connection-link"} {
		fixture := fixture
		t.Run(fixture, func(t *testing.T) {
			t.Parallel()
			directory := t.TempDir()
			source := rasterFixturePath(t, fixture+".d2")
			expected := mustReadRasterFixture(t, fixture+".exp.png")
			first := runRasterCLI(t, directory, source, "first.png", "--pad=16")
			second := runRasterCLI(t, directory, source, "second.png", "--pad=16")
			if !bytes.Equal(first, second) {
				t.Fatal("repeated PNG export changed deterministic PNG bytes")
			}
			if report, err := compareRasterImage(expected, first, "expected golden", "rendered output", "", fixture); err != nil {
				t.Fatalf("PNG fixture %q differs: %v; self-contained report: %s", fixture, err, report)
			}
		})
	}
}

func TestGIFFixtures(t *testing.T) {
	t.Parallel()

	for _, fixture := range []struct {
		intervalMS int
		frames     int
		durationCS int
	}{
		{intervalMS: 310, frames: 10, durationCS: 31},
		{intervalMS: 1050, frames: 32, durationCS: 105},
	} {
		fixture := fixture
		t.Run(fmt.Sprint(fixture.intervalMS), func(t *testing.T) {
			t.Parallel()
			directory := t.TempDir()
			source := rasterFixturePath(t, "animation.d2")
			expected := mustReadRasterFixture(t, fmt.Sprintf("animation-%d.exp.gif", fixture.intervalMS))
			arguments := []string{"--pad=16", fmt.Sprintf("--animate-interval=%d", fixture.intervalMS)}
			first := runRasterCLI(t, directory, source, "first.gif", arguments...)
			second := runRasterCLI(t, directory, source, "second.gif", arguments...)
			if !bytes.Equal(first, second) {
				t.Fatal("repeated GIF export changed deterministic GIF bytes")
			}
			comparison, err := testutil.CompareGIF(expected, first, testutil.GIFCompareOptions{RequireFrameChange: true})
			if err != nil {
				report := ""
				if comparison != nil && comparison.FrameResult != nil {
					report = writeRasterReport(t, comparison.FrameResult, "", fmt.Sprintf("gif-%d-frame-%d", fixture.intervalMS, comparison.FrameIndex))
				}
				t.Fatalf("GIF %dms differs: %v; frame report: %s", fixture.intervalMS, err, report)
			}
			inspection, err := testutil.InspectGIF(first)
			if err != nil {
				t.Fatal(err)
			}
			if len(inspection.FrameHashes) != fixture.frames ||
				inspection.TotalDurationCentiseconds != fixture.durationCS ||
				inspection.ChangedFramePairs == 0 {
				t.Fatalf("GIF %dms inspection = %+v", fixture.intervalMS, inspection)
			}
		})
	}
}

func runRasterCLI(t *testing.T, directory, source, output string, arguments ...string) []byte {
	t.Helper()
	environment := xos.NewEnv(os.Environ())
	args := append([]string(nil), arguments...)
	args = append(args, source, output)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := runTestMain(t, ctx, directory, environment, args...); err != nil {
		t.Fatalf("raster CLI: %v", err)
	}
	return readFile(t, directory, output)
}

func rasterFixturePath(t *testing.T, name string) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("testdata", "raster", name))
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func mustReadRasterFixture(t *testing.T, name string) []byte {
	t.Helper()
	content, err := os.ReadFile(rasterFixturePath(t, name))
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func compareRasterImage(expected, actual []byte, expectedName, actualName, reportRoot, reportName string) (string, error) {
	result, err := imagediff.Compare(expected, actual, imagediff.Options{ExpectedName: expectedName, ActualName: actualName})
	if err == nil {
		return "", nil
	}
	if result == nil {
		return "", err
	}
	return writeRasterReport(nil, result, reportRoot, reportName), err
}

func writeRasterReport(t *testing.T, result *imagediff.Result, reportRoot, reportName string) string {
	if reportRoot == "" {
		var err error
		reportRoot, err = os.MkdirTemp("", "d2-raster-diff-")
		if err != nil {
			if t != nil {
				t.Logf("create raster report directory: %v", err)
			}
			return ""
		}
	}
	reportName = strings.NewReplacer("/", "-", "\\", "-", " ", "-").Replace(reportName)
	path := filepath.Join(reportRoot, reportName+".html")
	if err := result.WriteReport(path); err != nil {
		if t != nil {
			t.Logf("write raster report: %v", err)
		}
		return ""
	}
	return path
}
