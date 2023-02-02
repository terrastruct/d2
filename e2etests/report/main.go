package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"io/ioutil"
	stdlog "log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"

	"oss.terrastruct.com/d2/lib/log"
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
	flag.BoolVar(&deltaFlag, "delta", false, "Generate the report only for cases that changed.")
	flag.StringVar(&testSetFlag, "test-set", "", "Only run set of tests matching this string. e.g. regressions")
	flag.StringVar(&testCaseFlag, "test-case", "", "Only run tests matching this string. e.g. all_shapes")
	skipTests := flag.Bool("skip-tests", false, "Skip running tests first")
	flag.BoolVar(&vFlag, "v", false, "verbose")
	flag.Parse()

	vString := ""
	if vFlag {
		vString = "-v"
	}
	testMatchString := fmt.Sprintf("TestE2E/%s/%s", testSetFlag, testCaseFlag)

	testDir := os.Getenv("TEST_DIR")
	if testDir == "" {
		testDir = "./e2etests"
	}

	if !*skipTests {
		ctx := log.Stderr(context.Background())
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, "go", "test", testDir, "-run", testMatchString, vString)
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
			files, err := ioutil.ReadDir(path)
			if err != nil {
				panic(err)
			}

			var testFile os.FileInfo
			for _, f := range files {
				if strings.HasSuffix(f.Name(), "exp.svg") {
					testFile = f
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
					fullPath := filepath.Join(path, testFile.Name())
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

		tmplData := TemplateData{
			Tests: tests,
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
		if err := tmpl.Execute(f, tmplData); err != nil {
			panic(err)
		}
	}
}
