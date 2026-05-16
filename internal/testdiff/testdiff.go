package testdiff

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/multierr"
	"oss.terrastruct.com/util-go/diff"
)

func TestdataJSON(path string, got interface{}) error {
	err := diff.TestdataJSON(path, got)
	return allowNewlineOnlyDiff(path, ".json", err)
}

func Testdata(path, ext string, got []byte) error {
	err := diff.Testdata(path, ext, got)
	return allowNewlineOnlyDiff(path, ext, err)
}

func TestdataTB(tb testing.TB, ext string, got []byte) {
	tb.Helper()
	if err := Testdata(filepath.Join("testdata", tb.Name()), ext, got); err != nil {
		tb.Fatal(err)
	}
}

func TestdataDir(tb testing.TB, dir string) {
	tb.Helper()
	err := testdataDir(filepath.Join("testdata", tb.Name()), dir)
	if err != nil {
		for _, err := range multierr.Errors(err) {
			tb.Error(err)
		}
	}
	if tb.Failed() {
		tb.FailNow()
	}
}

func testdataDir(testName, dir string) (err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		dirPath := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			err = multierr.Combine(err, testdataDir(filepath.Join(testName, entry.Name()), dirPath))
			continue
		}

		ext := filepath.Ext(entry.Name())
		name := strings.TrimSuffix(entry.Name(), ext)
		got, readErr := os.ReadFile(dirPath)
		if readErr != nil {
			err = multierr.Combine(err, readErr)
			continue
		}
		err = multierr.Combine(err, Testdata(filepath.Join(testName, name), ext, got))
	}
	return err
}

func allowNewlineOnlyDiff(path, ext string, err error) error {
	if err == nil {
		return nil
	}
	if os.Getenv("TESTDATA_ACCEPT") != "" || os.Getenv("TA") != "" {
		return err
	}
	if !strings.HasPrefix(err.Error(), "diff (rerun with ") {
		return err
	}

	expPath := fmt.Sprintf("%s.exp%s", path, ext)
	gotPath := fmt.Sprintf("%s.got%s", path, ext)

	exp, expErr := os.ReadFile(expPath)
	gotb, gotErr := os.ReadFile(gotPath)
	if expErr != nil || gotErr != nil {
		return err
	}

	if !bytes.Equal(normalizeNewlines(exp), normalizeNewlines(gotb)) {
		return err
	}

	if removeErr := os.Remove(gotPath); removeErr != nil {
		return removeErr
	}
	return nil
}

func normalizeNewlines(b []byte) []byte {
	return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
}
