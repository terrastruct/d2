package d2ir_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/d2lang/util-go/mapfs"

	"github.com/d2lang/d2/d2ir"
	"github.com/d2lang/d2/d2parser"
)

func benchmarkCompileSource(b *testing.B, source string) {
	b.Helper()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ast, err := d2parser.Parse("benchmark.d2", strings.NewReader(source), nil)
		if err != nil {
			b.Fatal(err)
		}
		if _, _, err := d2ir.Compile(ast, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCompileRepeatedImport(b *testing.B) {
	for _, count := range []int{10, 50, 100} {
		b.Run(fmt.Sprintf("imports=%d", count), func(b *testing.B) {
			var main strings.Builder
			var shared strings.Builder
			for i := 0; i < 100; i++ {
				fmt.Fprintf(&shared, "node%d: { style.fill: red }\n", i)
			}
			for i := 0; i < count; i++ {
				fmt.Fprintf(&main, "use%d: @shared.node%d\n", i, i%100)
			}
			filesystem, err := mapfs.New(map[string]string{
				"index.d2":  main.String(),
				"shared.d2": shared.String(),
			})
			if err != nil {
				b.Fatal(err)
			}
			defer filesystem.Close()

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ast, err := d2parser.Parse("index.d2", strings.NewReader(main.String()), nil)
				if err != nil {
					b.Fatal(err)
				}
				if _, _, err := d2ir.Compile(ast, &d2ir.CompileOptions{FS: filesystem}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkCompileFlatFields(b *testing.B) {
	for _, count := range []int{100, 1000, 4000} {
		b.Run(fmt.Sprintf("fields=%d", count), func(b *testing.B) {
			var source strings.Builder
			for i := 0; i < count; i++ {
				fmt.Fprintf(&source, "node%d\n", i)
			}
			benchmarkCompileSource(b, source.String())
		})
	}
}

func BenchmarkCompileLeadingGlob(b *testing.B) {
	for _, count := range []int{100, 500, 1000, 4000} {
		b.Run(fmt.Sprintf("fields=%d", count), func(b *testing.B) {
			var source strings.Builder
			source.WriteString("**.style.stroke-width: 2\n")
			for i := 0; i < count; i++ {
				fmt.Fprintf(&source, "node%d\n", i)
			}
			benchmarkCompileSource(b, source.String())
		})
	}
}

func BenchmarkCompileDistinctEdges(b *testing.B) {
	for _, count := range []int{100, 1000, 3200} {
		b.Run(fmt.Sprintf("edges=%d", count), func(b *testing.B) {
			var source strings.Builder
			for i := 0; i <= count; i++ {
				fmt.Fprintf(&source, "node%d\n", i)
			}
			for i := 0; i < count; i++ {
				fmt.Fprintf(&source, "node%d -> node%d\n", i, i+1)
			}
			benchmarkCompileSource(b, source.String())
		})
	}
}
