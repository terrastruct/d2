package d2dagrelayout

import (
	"os"
	"path/filepath"
	"testing"
)

// Keep layout measurements separate from parsing and font measurement, using
// the same real-world sources as the D2 end-to-end output compatibility suite.
func BenchmarkDagreRealWorld(b *testing.B) {
	for _, name := range []string{"tpmjs_architecture", "spyre_encoder", "mocha_soc", "fulcro_rad", "queue_workers"} {
		b.Run(name, func(b *testing.B) {
			source, err := os.ReadFile(filepath.Join("..", "..", "e2etests", "testdata", "files", name+".d2"))
			if err != nil {
				b.Fatal(err)
			}
			benchmarkDagreLayout(b, string(source))
		})
	}
}
