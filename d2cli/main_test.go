package d2cli_test

import "testing"

// TODO: Use virtual file system with fs.FS in xmain.State when fs.FS supports writes.
// See https://github.com/golang/go/issues/45757. Or modify.
// We would need to abstract out fsnotify as well to work with the virtual test file system.
// See also https://pkg.go.dev/testing/fstest
// For now cleaning up temp directories after running tests is enough.
func TestRun(t *testing.T) {
	t.Parallel()
}
