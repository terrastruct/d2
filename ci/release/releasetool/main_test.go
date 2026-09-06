package main

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

func TestArchiveIsReproducibleAndNormalized(t *testing.T) {
	t.Parallel()
	stamp := time.Unix(1_700_000_000, 0).UTC()
	base := t.TempDir()
	rootA := filepath.Join(base, "a", "d2-v1.2.3")
	rootB := filepath.Join(base, "b", "d2-v1.2.3")
	populateReleaseTree(t, rootA, time.Unix(10, 0))
	populateReleaseTree(t, rootB, time.Unix(20, 0))
	archiveA := filepath.Join(base, "a.tar.gz")
	archiveB := filepath.Join(base, "b.tar.gz")
	if err := writeArchive(rootA, archiveA, stamp); err != nil {
		t.Fatal(err)
	}
	if err := writeArchive(rootB, archiveB, stamp); err != nil {
		t.Fatal(err)
	}
	bytesA, err := os.ReadFile(archiveA)
	if err != nil {
		t.Fatal(err)
	}
	bytesB, err := os.ReadFile(archiveB)
	if err != nil {
		t.Fatal(err)
	}
	if string(bytesA) != string(bytesB) {
		t.Fatal("equivalent release trees produced different archives")
	}
	if err := verifyArchive(archiveA, "d2-v1.2.3", stamp, int64(len(bytesA))); err != nil {
		t.Fatal(err)
	}

	file, err := os.Open(archiveA)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	zr, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	tr := tar.NewReader(zr)
	var names []string
	for {
		header, err := tr.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatal(err)
		}
		names = append(names, header.Name)
		if header.Name == "d2-v1.2.3/bin/d2" && header.Mode != 0o755 {
			t.Fatalf("binary mode is %#o", header.Mode)
		}
	}
	if got := strings.Join(names, "\n"); !strings.Contains(got, "d2-v1.2.3/scripts/install.sh") {
		t.Fatalf("archive names do not contain installer:\n%s", got)
	}
}

func TestValidateSymlink(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		target string
		ok     bool
	}{
		{name: "d2-v1.2.3/scripts/d2", target: "../bin/d2", ok: true},
		{name: "d2-v1.2.3/d2", target: "bin/d2", ok: true},
		{name: "d2-v1.2.3/d2", target: "/usr/bin/d2"},
		{name: "d2-v1.2.3/d2", target: `C:\\Windows\\d2.exe`},
		{name: "d2-v1.2.3/scripts/d2", target: "../../outside"},
		{name: "d2-v1.2.3/scripts/d2", target: ""},
	} {
		err := validateSymlink(test.name, test.target, "d2-v1.2.3")
		if (err == nil) != test.ok {
			t.Errorf("validateSymlink(%q, %q) error = %v, want ok=%v", test.name, test.target, err, test.ok)
		}
	}
}

func TestArchiveRejectsOutputInsideRoot(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "d2-v1.2.3")
	populateReleaseTree(t, root, time.Now())
	err := writeArchive(root, filepath.Join(root, "release.tar.gz"), time.Unix(0, 0))
	if err == nil || !strings.Contains(err.Error(), "must not be inside") {
		t.Fatalf("writeArchive error = %v", err)
	}
}

func TestChecksumsAreSortedAndVerified(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	a := filepath.Join(directory, "a.tar.gz")
	b := filepath.Join(directory, "b.tar.gz")
	if err := os.WriteFile(b, []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(a, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(directory, "SHA256SUMS")
	if err := writeChecksums(manifest, []string{b, a}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 || !strings.HasSuffix(lines[0], "  a.tar.gz") || !strings.HasSuffix(lines[1], "  b.tar.gz") {
		t.Fatalf("manifest is not sorted:\n%s", data)
	}
	if err := verifyChecksums(manifest, directory, 2); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(a, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyChecksums(manifest, directory, 2); err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("verifyChecksums error = %v", err)
	}
}

func TestChecksumsRejectUnsafeSubjects(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	manifest := filepath.Join(directory, "SHA256SUMS")
	line := strings.Repeat("0", 64) + "  ../escape\n"
	if err := os.WriteFile(manifest, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyChecksums(manifest, directory, 1); err == nil || !strings.Contains(err.Error(), "invalid checksum subject") {
		t.Fatalf("verifyChecksums error = %v", err)
	}
}

func TestSPDXDocumentIsDeterministicAndSorted(t *testing.T) {
	t.Parallel()
	build := &debug.BuildInfo{
		GoVersion: "go1.27.0",
		Path:      mainModule,
		Settings: []debug.BuildSetting{{
			Key:   "vcs.revision",
			Value: strings.Repeat("a", 40),
		}},
		Deps: []*debug.Module{
			{Path: "example.com/z", Version: "v1.0.0", Sum: "h1:z"},
			{Path: "example.com/a", Version: "v2.0.0", Sum: "h1:a"},
		},
	}
	stamp := time.Unix(1_700_000_000, 0).UTC()
	document, err := newSPDXDocument(build, "d2-v1.2.3", "v1.2.3", stamp)
	if err != nil {
		t.Fatal(err)
	}
	if document.SPDXVersion != "SPDX-2.3" || document.CreationInfo.Created != "2023-11-14T22:13:20Z" {
		t.Fatalf("unexpected SPDX metadata: %#v", document)
	}
	if len(document.Packages) != 3 || document.Packages[1].Name != "example.com/a" || document.Packages[2].Name != "example.com/z" {
		t.Fatalf("SPDX packages are not sorted: %#v", document.Packages)
	}
	if len(document.Relationships) != 3 || document.Relationships[1].Type != "DEPENDS_ON" {
		t.Fatalf("unexpected SPDX relationships: %#v", document.Relationships)
	}
	if got := document.Packages[1].ExternalReferences[0].Locator; got != "pkg:golang/example.com/a@v2.0.0" {
		t.Fatalf("package URL = %q", got)
	}
}

func populateReleaseTree(t *testing.T, root string, mtime time.Time) {
	t.Helper()
	files := map[string]struct {
		contents string
		mode     os.FileMode
	}{
		"LICENSE.txt":             {contents: "license\n", mode: 0o644},
		"THIRD_PARTY_NOTICES.txt": {contents: "notices\n", mode: 0o644},
		"Makefile":                {contents: "all:\n\t@true\n", mode: 0o644},
		"README.md":               {contents: "readme\n", mode: 0o644},
		"bin/d2":                  {contents: "binary\n", mode: 0o755},
		"man/d2.1":                {contents: "manpage\n", mode: 0o644},
		"scripts/install.sh":      {contents: "#!/bin/sh\n", mode: 0o755},
		"scripts/lib.sh":          {contents: "#!/bin/sh\n", mode: 0o755},
		"scripts/uninstall.sh":    {contents: "#!/bin/sh\n", mode: 0o755},
	}
	for name, file := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(file.contents), file.mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
	if runtime.GOOS != "windows" {
		link := filepath.Join(root, "scripts", "d2-link")
		if err := os.Symlink("../bin/d2", link); err != nil {
			t.Fatal(err)
		}
	}
}
