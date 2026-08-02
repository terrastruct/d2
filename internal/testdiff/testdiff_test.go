package testdiff

import (
	"os"
	"path/filepath"
	"testing"
)

type testValue struct {
	Name string `json:"name"`
}

func TestTestdataJSONAllowsOnlyNewlineDiff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "case")
	expPath := path + ".exp.json"
	gotPath := path + ".got.json"

	err := os.WriteFile(expPath, []byte("{\r\n  \"name\": \"ok\"\r\n}\r\n"), 0600)
	if err != nil {
		t.Fatal(err)
	}

	err = TestdataJSON(path, testValue{Name: "ok"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(gotPath); !os.IsNotExist(err) {
		t.Fatalf("expected got file to be removed after newline-only diff, got err=%v", err)
	}
}

func TestTestdataJSONKeepsRealDiff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "case")
	expPath := path + ".exp.json"
	gotPath := path + ".got.json"

	err := os.WriteFile(expPath, []byte("{\r\n  \"name\": \"old\"\r\n}\r\n"), 0600)
	if err != nil {
		t.Fatal(err)
	}

	err = TestdataJSON(path, testValue{Name: "new"})
	if err == nil {
		t.Fatal("expected real JSON diff to fail")
	}

	if _, err := os.Stat(gotPath); err != nil {
		t.Fatalf("expected got file to remain after real diff, got err=%v", err)
	}
}

func TestTestdataJSONPreservesAcceptErrors(t *testing.T) {
	t.Setenv("TA", "1")

	path := filepath.Join(t.TempDir(), "case")
	expPath := path + ".exp.json"
	gotPath := path + ".got.json"

	err := os.Mkdir(expPath, 0700)
	if err != nil {
		t.Fatal(err)
	}

	err = TestdataJSON(path, testValue{Name: "ok"})
	if err == nil {
		t.Fatal("expected accept-mode rename error to be preserved")
	}

	if _, err := os.Stat(gotPath); err != nil {
		t.Fatalf("expected got file to remain after accept error, got err=%v", err)
	}
}

func TestTestdataAllowsOnlyNewlineDiff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "case")
	expPath := path + ".exp.txt"
	gotPath := path + ".got.txt"

	err := os.WriteFile(expPath, []byte("line 1\r\nline 2\r\n"), 0600)
	if err != nil {
		t.Fatal(err)
	}

	err = Testdata(path, ".txt", []byte("line 1\nline 2\n"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(gotPath); !os.IsNotExist(err) {
		t.Fatalf("expected got file to be removed after newline-only diff, got err=%v", err)
	}
}

func TestTestdataKeepsRealDiff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "case")
	expPath := path + ".exp.svg"
	gotPath := path + ".got.svg"

	err := os.WriteFile(expPath, []byte("<svg>\r\n  <text>old</text>\r\n</svg>\r\n"), 0600)
	if err != nil {
		t.Fatal(err)
	}

	err = Testdata(path, ".svg", []byte("<svg>\n  <text>new</text>\n</svg>\n"))
	if err == nil {
		t.Fatal("expected real SVG diff to fail")
	}

	if _, err := os.Stat(gotPath); err != nil {
		t.Fatalf("expected got file to remain after real diff, got err=%v", err)
	}
}
