package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	stdlog "log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"

	"oss.terrastruct.com/d2/lib/log"
	timelib "oss.terrastruct.com/d2/lib/time"
)

//go:embed template.html
var TEMPLATE_HTML string

type TemplateData struct {
	Tests []TestItem
}

type TestItem struct {
	Name   string
	ExpSVG *string
	GotSVG string
}

func main() {
	deltaFlag := false
	vFlag := false
	testCaseFlag := ""
	testSetFlag := ""
	testNameFlag := ""
	cpuProfileFlag := false
	memProfileFlag := false
	flag.BoolVar(&deltaFlag, "delta", false, "Generate the report only for cases that changed.")
	flag.StringVar(&testNameFlag, "test-name", "E2E", "Name of e2e tests. Defaults to E2E")
	flag.StringVar(&testSetFlag, "test-set", "", "Only run set of tests matching this string. e.g. regressions")
	flag.StringVar(&testCaseFlag, "test-case", "", "Only run tests matching this string. e.g. all_shapes")
	flag.BoolVar(&cpuProfileFlag, "cpuprofile", false, "Profile test cpu usage. `go tool pprof out/cpu.prof`")
	flag.BoolVar(&memProfileFlag, "memprofile", false, "Profile test memory usage. `go tool pprof out/mem.prof`")
	skipTests := flag.Bool("skip-tests", false, "Skip running tests first")
	flag.BoolVar(&vFlag, "v", false, "verbose")
	flag.Parse()

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

		ctx, cancel := timelib.WithTimeout(ctx, 2*time.Minute)
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

	var tests []TestItem
	err := filepath.Walk(filepath.Join(testDir, "testdata"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			files, err := os.ReadDir(path)
			if err != nil {
				panic(err)
			}

			var testFile os.FileInfo
			for _, f := range files {
				if strings.HasSuffix(f.Name(), "exp.svg") {
					testFile, _ = f.Info()
					break
				}
			}

			if testFile != nil {
				testCaseRoot := filepath.Dir(path)
				matchTestCase := true
				if testCaseFlag != "" {
					matchTestCase, _ = regexp.MatchString(testCaseFlag, filepath.Base(testCaseRoot))
				}
				matchTestSet := true
				if testSetFlag != "" {
					matchTestSet, _ = regexp.MatchString(testSetFlag, filepath.Base(filepath.Dir(testCaseRoot)))
				}

				if matchTestSet && matchTestCase {
					absPath, err := filepath.Abs(path)
					if err != nil {
						stdlog.Fatal(err)
					}
					fullPath := filepath.Join(absPath, testFile.Name())
					hasGot := false
					gotPath := strings.Replace(fullPath, "exp.svg", "got.svg", 1)
					if _, err := os.Stat(gotPath); err == nil {
						hasGot = true
					}
					// e.g. arrowhead_adjustment/dagre
					name := filepath.Join(filepath.Base(testCaseRoot), info.Name())
					if deltaFlag {
						if hasGot {
							tests = append(tests, TestItem{
								Name:   name,
								ExpSVG: &fullPath,
								GotSVG: gotPath,
							})
						}
					} else {
						test := TestItem{
							Name:   name,
							ExpSVG: nil,
							GotSVG: fullPath,
						}
						if hasGot {
							test.GotSVG = gotPath
						}
						tests = append(tests, test)
					}
				}
			}
		}
		return nil
	},
	)
	if err != nil {
		panic(err)
	}

	if len(tests) > 0 {
		tmpl, err := template.New("report").Parse(TEMPLATE_HTML)
		if err != nil {
			panic(err)
		}

		path := os.Getenv("REPORT_OUTPUT")
		if path == "" {
			path = filepath.Join(testDir, "./out/e2e_report.html")
		}
		err = os.MkdirAll(filepath.Dir(path), 0755)
		if err != nil {
			stdlog.Fatal(err)
		}
		f, err := os.Create(path)
		if err != nil {
			panic(fmt.Errorf("error creating file `%s`. %v", path, err))
		}
		absReportDir, err := filepath.Abs(filepath.Dir(path))
		if err != nil {
			stdlog.Fatal(err)
		}

		// get the test path relative to the report
		reportRelPath := func(testPath string) string {
			relTestPath, err := filepath.Rel(absReportDir, testPath)
			if err != nil {
				stdlog.Fatal(err)
			}
			return relTestPath
		}

		// update test paths to be relative to report file
		for i := range tests {
			testItem := &tests[i]
			testItem.GotSVG = reportRelPath(testItem.GotSVG)
			if testItem.ExpSVG != nil {
				*testItem.ExpSVG = reportRelPath(*testItem.ExpSVG)
			}
		}

		if err := tmpl.Execute(f, TemplateData{Tests: tests}); err != nil {
			panic(err)
		}
	}
}
