package d2cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestIsometricAssetsHonorPrivateNetworkPolicy(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "image/svg+xml")
		_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20"><rect width="20" height="20" fill="red"/></svg>`))
	}))
	defer server.Close()
	for _, format := range []string{"svg", "png", "pdf"} {
		t.Run(format, func(t *testing.T) {
			source := "a: {icon: " + server.URL + "/" + format + "/icon.svg}"
			before := requests.Load()
			args := []string{"--isometric", "--scale=.1", "--stdout-format=" + format, "-"}
			if data, err := imageCLI(t, t.TempDir(), source, args...); err == nil || len(data) != 0 {
				t.Fatalf("private asset should fail before output: bytes=%d, error=%v", len(data), err)
			}
			if requests.Load() != before {
				t.Fatal("default policy fetched a private-network asset")
			}
			args = append([]string{"--allow-private-network"}, args...)
			if data, err := imageCLI(t, t.TempDir(), source, args...); err != nil || len(data) == 0 {
				t.Fatalf("explicit private-network permission failed: bytes=%d, error=%v", len(data), err)
			}
			if requests.Load() <= before {
				t.Fatal("explicit private-network permission did not fetch the asset")
			}
		})
	}
}

func TestIsometricMultiboardPublicationIsTransactional(t *testing.T) {
	dir := t.TempDir()
	args := []string{"--isometric", "--scale=.1", "views.svg"}
	if _, err := imageCLI(t, dir, "a\nlayers: {old: {b}}", args...); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(dir, "views", "index.svg")
	previous, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	unrelated := filepath.Join(dir, "views", "notes.txt")
	if err := os.WriteFile(unrelated, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := imageCLI(t, dir, "changed\nlayers: {broken: {b: {icon: ./missing.png}}}", args...); err == nil {
		t.Fatal("missing child asset unexpectedly rendered")
	}
	if got, err := os.ReadFile(indexPath); err != nil || !bytes.Equal(got, previous) {
		t.Fatalf("failed child render changed previous root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "views", "old.svg")); err != nil {
		t.Fatalf("failed render removed previous child: %v", err)
	}
	if _, err := imageCLI(t, dir, "changed\nlayers: {new: {b}}", args...); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "views", "old.svg")); !os.IsNotExist(err) {
		t.Fatalf("successful render retained stale generated child: %v", err)
	}
	if got, err := os.ReadFile(unrelated); err != nil || string(got) != "keep" {
		t.Fatalf("publication changed unrelated file: %v", err)
	}
	stages, err := filepath.Glob(filepath.Join(dir, ".d2-board-output-*"))
	if err != nil || len(stages) != 0 {
		t.Fatalf("publication left staging directories: %v, %v", stages, err)
	}
}

func TestIsometricMultiboardEscapesUnsafeNames(t *testing.T) {
	dir := t.TempDir()
	source := `a
layers: {"../escaped": {b}}`
	if _, err := imageCLI(t, dir, source, "--isometric", "--scale=.1", "views.svg"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "views", boardOutputComponent("../escaped")+".svg")); err != nil {
		t.Fatalf("unsafe name was not mapped to a safe component: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "escaped.svg")); !os.IsNotExist(err) {
		t.Fatalf("board escaped the output directory: %v", err)
	}
}
