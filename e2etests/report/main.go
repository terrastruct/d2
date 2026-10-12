package main

import (
	"bytes"
	"context"
	_ "embed"
	"flag"
	"fmt"
	"html/template"
	stdlog "log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/d2lang/d2/lib/log"
	timelib "github.com/d2lang/d2/lib/time"
)

//go:embed template.html
var TEMPLATE_HTML string

type TemplateData struct {
	Tests []TestItem
}

type TestItem struct {
	ID, Name, Variant  string
	ExpImage, GotImage string
	GotLabel           string
	MissingExpected    bool
}

type discoveryOptions struct {
	Delta                      bool
	Variant, TestSet, TestCase string
}

// Discover each SVG snapshot independently, including multiple isometric boards.
// PNG coverage lives in a separate test bundle. Got-only images are useful
// before admitting goldens.
func discoverTests(testdata string, opts discoveryOptions) ([]TestItem, error) {
	if opts.Variant != "all" && opts.Variant != "sketch" && opts.Variant != "isometric" {
		return nil, fmt.Errorf("invalid variant %q: use all, sketch or isometric", opts.Variant)
	}
	setRE, err := regexp.Compile(opts.TestSet)
	if err != nil {
		return nil, fmt.Errorf("invalid test-set: %w", err)
	}
	caseRE, err := regexp.Compile(opts.TestCase)
	if err != nil {
		return nil, fmt.Errorf("invalid test-case: %w", err)
	}
	root, err := filepath.Abs(testdata)
	if err != nil {
		return nil, err
	}
	type pair struct{ name, variant, exp, got string }
	pairs := make(map[string]*pair)
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		stem, kind, ext, ok := snapshotName(entry.Name())
		if !ok {
			return nil
		}
		variant := "sketch"
		if stem == "isometric" || strings.HasPrefix(stem, "isometric.") {
			variant = "isometric"
		}
		if opts.Variant != "all" && opts.Variant != variant {
			return nil
		}
		dir := filepath.Dir(path)
		relative, err := filepath.Rel(root, dir)
		if err != nil {
			return err
		}
		// Sets are rooted at testdata/<set>. ASCII fixtures have no layout
		// directory; other cases may include slash-separated subtest names.
		parts := strings.Split(filepath.ToSlash(relative), "/")
		if len(parts) < 2 {
			return nil
		}
		setName := parts[0]
		caseParts := parts[1:]
		if setName != "asciitxtar" && len(caseParts) > 1 {
			switch caseParts[len(caseParts)-1] {
			case "dagre", "elk":
				caseParts = caseParts[:len(caseParts)-1]
			}
		}
		if !setRE.MatchString(setName) || !caseRE.MatchString(strings.Join(caseParts, "/")) {
			return nil
		}
		key := filepath.Join(dir, stem+ext)
		item := pairs[key]
		if item == nil {
			item = &pair{name: filepath.ToSlash(filepath.Join(relative, stem+ext)), variant: variant}
			pairs[key] = item
		}
		if kind == "exp" {
			item.exp = path
		} else {
			item.got = path
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	var tests []TestItem
	for _, pair := range pairs {
		changed := pair.exp == ""
		if pair.exp != "" && pair.got != "" {
			exp, err := os.ReadFile(pair.exp)
			if err != nil {
				return nil, err
			}
			got, err := os.ReadFile(pair.got)
			if err != nil {
				return nil, err
			}
			changed = !bytes.Equal(exp, got)
		}
		if opts.Delta && !changed {
			continue
		}
		item := TestItem{Name: pair.name, Variant: pair.variant, MissingExpected: pair.exp == ""}
		if changed {
			item.ExpImage, item.GotImage, item.GotLabel = pair.exp, pair.got, "Got"
		} else {
			item.GotImage, item.GotLabel = pair.exp, "Expected"
		}
		tests = append(tests, item)
	}
	sort.Slice(tests, func(i, j int) bool { return tests[i].Name < tests[j].Name })
	for i := range tests {
		tests[i].ID = fmt.Sprintf("test-%d", i+1)
	}
	return tests, nil
}

func snapshotName(name string) (stem, kind, ext string, ok bool) {
	for _, kind := range []string{"exp", "got"} {
		suffix := "." + kind + ".svg"
		if !strings.HasSuffix(name, suffix) {
			continue
		}
		stem := strings.TrimSuffix(name, suffix)
		if stem == "" || stem == "isometric." {
			continue
		}
		return stem, kind, ".svg", true
	}
	return "", "", "", false
}

func imageURL(reportDir, imagePath string) (string, error) {
	if imagePath == "" {
		return "", nil
	}
	relative, err := filepath.Rel(reportDir, imagePath)
	if err != nil {
		return "", err
	}
	return (&url.URL{Path: filepath.ToSlash(relative)}).String(), nil
}

func writeReport(path string, tests []TestItem) error {
	dir, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return err
	}
	tests = append([]TestItem(nil), tests...)
	for i := range tests {
		tests[i].ExpImage, err = imageURL(dir, tests[i].ExpImage)
		if err != nil {
			return err
		}
		tests[i].GotImage, err = imageURL(dir, tests[i].GotImage)
		if err != nil {
			return err
		}
	}
	tmpl, err := template.New("report").Parse(TEMPLATE_HTML)
	if err != nil {
		return err
	}
	var output bytes.Buffer
	if err := tmpl.Execute(&output, TemplateData{Tests: tests}); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(path, output.Bytes(), 0644)
}

func main() {
	deltaFlag := false
	vFlag := false
	testCaseFlag := ""
	testSetFlag := ""
	testNameFlag := ""
	variantFlag := "all"
	testTimeout := 10 * time.Minute
	cpuProfileFlag := false
	memProfileFlag := false
	flag.DurationVar(&testTimeout, "timeout", 10*time.Minute, "Timeout for running e2e tests before generating the report.")
	flag.StringVar(&variantFlag, "variant", "all", "Snapshot variants to display: all, sketch or isometric.")
	flag.BoolVar(&deltaFlag, "delta", false, "Generate the report only for cases that changed.")
	flag.StringVar(&testNameFlag, "test-name", "E2E", "Name of e2e tests. Defaults to E2E")
	flag.StringVar(&testSetFlag, "test-set", "", "Only run set of tests matching this string. e.g. regressions")
	flag.StringVar(&testCaseFlag, "test-case", "", "Only run tests matching this string. e.g. all_shapes")
	flag.BoolVar(&cpuProfileFlag, "cpuprofile", false, "Profile test cpu usage. `go tool pprof out/cpu.prof`")
	flag.BoolVar(&memProfileFlag, "memprofile", false, "Profile test memory usage. `go tool pprof out/mem.prof`")
	skipTests := flag.Bool("skip-tests", false, "Skip running tests first")
	flag.BoolVar(&vFlag, "v", false, "verbose")
	flag.Parse()
	if variantFlag != "all" && variantFlag != "sketch" && variantFlag != "isometric" {
		stdlog.Fatal("invalid -variant: use all, sketch or isometric")
	}

	vString := ""
	if vFlag {
		vString = "-v"
	}
	testMatchString := fmt.Sprintf("-run=Test%s/%s/%s", testNameFlag, testSetFlag, testCaseFlag)

	cpuProfileStr := ""
	if cpuProfileFlag {
		cpuProfileStr = `-cpuprofile=out/cpu.prof`
	}
	memProfileStr := ""
	if memProfileFlag {
		memProfileStr = `-memprofile=out/mem.prof`
	}

	testDir := os.Getenv("TEST_DIR")
	if testDir == "" {
		testDir = "./e2etests"
	}

	if !*skipTests {
		ctx := context.Background()

		ctx, cancel := timelib.WithTimeout(ctx, testTimeout)
		defer cancel()

		// don't want to pass empty args to CommandContext
		args := []string{"test", testDir, testMatchString}
		if cpuProfileStr != "" {
			args = append(args, cpuProfileStr)
		}
		if memProfileStr != "" {
			args = append(args, memProfileStr)
		}
		if vString != "" {
			args = append(args, vString)
		}
		cmd := exec.CommandContext(ctx, "go", args...)
		cmd.Env = os.Environ()
		cmd.Env = append(cmd.Env, "FORCE_COLOR=1")
		cmd.Env = append(cmd.Env, "DEBUG=1")
		cmd.Env = append(cmd.Env, "TEST_MODE=on")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		log.Debug(ctx, cmd.String())
		_ = cmd.Run()
	}

	tests, err := discoverTests(filepath.Join(testDir, "testdata"), discoveryOptions{
		Delta: deltaFlag, Variant: variantFlag, TestSet: testSetFlag, TestCase: testCaseFlag,
	})
	if err != nil {
		stdlog.Fatal(err)
	}
	path := os.Getenv("REPORT_OUTPUT")
	if path == "" {
		path = filepath.Join(testDir, "out/e2e_report.html")
	}
	if err := writeReport(path, tests); err != nil {
		stdlog.Fatal(err)
	}
	fmt.Printf("Wrote %d snapshots to %s\n", len(tests), path)
}
