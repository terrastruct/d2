package fontface

import (
	"os"
	"path/filepath"
	"testing"
)

func testFontData(t testing.TB, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "d2fonts", "ttf", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}
