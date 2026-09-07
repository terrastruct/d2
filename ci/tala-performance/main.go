// Command tala-performance measures TALA callbacks on an external D2 corpus and
// fingerprints complete layout outputs. It never updates tests or goldens.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2layouts/d2talalayout"
	"github.com/d2lang/d2/d2lib"
	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2renderers/d2svg"
	d2log "github.com/d2lang/d2/lib/log"
	"github.com/d2lang/d2/lib/textmeasure"
)

// caseInput is deliberately independent of any private repository. Source and
// licensed font data remain in caller-supplied files outside the D2 tree.
type caseInput struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	Seed   int64  `json:"seed"`
	Sketch bool   `json:"sketch,omitempty"`
	Theme  *int64 `json:"theme,omitempty"`
	Skip   bool   `json:"skip,omitempty"`
}

type result struct {
	Name          string `json:"name"`
	Iteration     int    `json:"iteration"`
	InputSHA256   string `json:"input_sha256"`
	CompileNS     int64  `json:"compile_ns"`
	LayoutNS      int64  `json:"layout_ns"`
	RoutingNS     int64  `json:"routing_ns"`
	RenderNS      int64  `json:"render_ns"`
	RerouteNS     int64  `json:"reroute_ns,omitempty"`
	LayoutCalls   int    `json:"layout_calls"`
	RoutingCalls  int    `json:"routing_calls"`
	GraphSHA256   string `json:"graph_sha256,omitempty"`
	DiagramSHA256 string `json:"diagram_sha256,omitempty"`
	SVGSHA256     string `json:"svg_sha256,omitempty"`
	RerouteSHA256 string `json:"reroute_sha256,omitempty"`
	Error         string `json:"error,omitempty"`
}

func hash(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Include every board, since SerializeGraph itself represents one board only.
func graphBytes(g *d2graph.Graph) ([]byte, error) {
	board, err := d2graph.SerializeGraph(g)
	if err != nil {
		return nil, err
	}
	out := struct {
		Board     json.RawMessage   `json:"board"`
		Layers    []json.RawMessage `json:"layers"`
		Scenarios []json.RawMessage `json:"scenarios"`
		Steps     []json.RawMessage `json:"steps"`
	}{Board: board}
	for i, children := range [][]*d2graph.Graph{g.Layers, g.Scenarios, g.Steps} {
		for _, child := range children {
			b, err := graphBytes(child)
			if err != nil {
				return nil, err
			}
			switch i {
			case 0:
				out.Layers = append(out.Layers, b)
			case 1:
				out.Scenarios = append(out.Scenarios, b)
			case 2:
				out.Steps = append(out.Steps, b)
			}
		}
	}
	return json.Marshal(out)
}

func runCase(ctx context.Context, tc caseInput, iteration int, font *d2fonts.FontFamily, reroute bool, artifactDir string) (r result) {
	r.Name, r.Iteration = tc.Name, iteration
	input, err := json.Marshal(tc)
	if err != nil {
		r.Error = err.Error()
		return
	}
	r.InputSHA256 = hash(input)
	fail := func(err error) bool {
		if err != nil {
			r.Error = err.Error()
			return true
		}
		return false
	}
	ruler, err := textmeasure.NewRuler()
	if fail(err) {
		return
	}
	if tc.Sketch {
		font = nil
	}
	engine, mono, scale, omit := "tala", d2fonts.SourceCodePro, 1.0, true
	compileOpts := &d2lib.CompileOptions{
		UTF16Pos: true, Ruler: ruler, FontFamily: font, MonoFontFamily: &mono, Layout: &engine,
		LayoutResolver: func(string) (d2graph.LayoutGraph, error) {
			return func(ctx context.Context, g *d2graph.Graph) (err error) {
				pprof.Do(ctx, pprof.Labels("phase", "layout"), func(ctx context.Context) {
					start := time.Now()
					err = d2talalayout.Layout(ctx, g, &d2talalayout.Options{Seeds: []int64{tc.Seed}, MaxConcurrency: 1})
					r.LayoutNS += time.Since(start).Nanoseconds()
					r.LayoutCalls++
				})
				return err
			}, nil
		},
		RouterResolver: func(string) (d2graph.RouteEdges, error) {
			return func(ctx context.Context, g *d2graph.Graph, edges []*d2graph.Edge) (err error) {
				pprof.Do(ctx, pprof.Labels("phase", "routing"), func(ctx context.Context) {
					start := time.Now()
					err = d2talalayout.RouteEdges(ctx, g, edges)
					r.RoutingNS += time.Since(start).Nanoseconds()
					r.RoutingCalls++
				})
				return err
			}, nil
		},
	}
	renderOpts := &d2svg.RenderOpts{Sketch: &tc.Sketch, Scale: &scale, ThemeID: tc.Theme, OmitVersion: &omit}
	start := time.Now()
	diagram, graph, err := d2lib.Compile(ctx, tc.Source, compileOpts, renderOpts)
	r.CompileNS = time.Since(start).Nanoseconds()
	if fail(err) {
		return
	}
	gb, err := graphBytes(graph)
	if fail(err) {
		return
	}
	r.GraphSHA256 = hash(gb)
	db, err := json.Marshal(diagram)
	if fail(err) {
		return
	}
	r.DiagramSHA256 = hash(db)
	start = time.Now()
	svg, err := d2svg.Render(diagram, renderOpts)
	r.RenderNS = time.Since(start).Nanoseconds()
	if fail(err) {
		return
	}
	r.SVGSHA256 = hash(svg)
	if artifactDir != "" {
		// Hash filenames avoid trusting corpus names as filesystem paths.
		stem := filepath.Join(artifactDir, hash([]byte(tc.Name)))
		for _, file := range []struct {
			suffix string
			data   []byte
		}{{".graph.json", gb}, {".diagram.json", db}, {".svg", svg}} {
			if fail(os.WriteFile(stem+file.suffix, file.data, 0600)) {
				return
			}
		}
	}
	if reroute {
		pprof.Do(ctx, pprof.Labels("phase", "reroute"), func(ctx context.Context) {
			start := time.Now()
			err = d2talalayout.RouteEdges(ctx, graph, graph.Edges)
			r.RerouteNS = time.Since(start).Nanoseconds()
		})
		if fail(err) {
			return
		}
		gb, err = graphBytes(graph)
		if fail(err) {
			return
		}
		r.RerouteSHA256 = hash(gb)
	}
	return r
}

func main() {
	corpus := flag.String("corpus", "", "external JSON array of {name, source, seed, sketch, theme, skip}")
	output := flag.String("output", "", "JSONL results path (default stdout)")
	count := flag.Int("count", 1, "complete sequential corpus repetitions")
	filter := flag.String("filter", ".", "regular expression matching case names")
	regular := flag.String("font-regular", "", "optional external regular TTF")
	bold := flag.String("font-bold", "", "optional external bold TTF (requires regular)")
	fontName := flag.String("font-name", "d2Font", "family name for external font")
	reroute := flag.Bool("reroute", false, "also measure a separate reroute of the root board after compilation")
	profile := flag.String("cpuprofile", "", "write CPU profile; filter with pprof -tagfocus=phase=layout")
	memprofile := flag.String("memprofile", "", "write allocation profile after the run")
	artifacts := flag.String("artifacts", "", "optional private output directory for complete graph, diagram, and SVG files")
	flag.Parse()
	if *corpus == "" || *count < 1 {
		flag.Usage()
		os.Exit(2)
	}
	die := func(err error) {
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	b, err := os.ReadFile(*corpus)
	die(err)
	var cases []caseInput
	die(json.Unmarshal(b, &cases))
	match, err := regexp.Compile(*filter)
	die(err)
	seen := map[string]bool{}
	for _, tc := range cases {
		if tc.Name == "" || seen[tc.Name] {
			die(fmt.Errorf("empty or duplicate case name %q", tc.Name))
		}
		seen[tc.Name] = true
	}
	var font *d2fonts.FontFamily
	if *regular != "" {
		r, err := os.ReadFile(*regular)
		die(err)
		bf := r
		if *bold != "" {
			bf, err = os.ReadFile(*bold)
			die(err)
		}
		font, err = d2fonts.AddFontFamily(*fontName, r, r, bf, bf)
		die(err)
	} else if *bold != "" {
		die(fmt.Errorf("-font-bold requires -font-regular"))
	}
	if *artifacts != "" {
		die(os.MkdirAll(*artifacts, 0700))
	}
	var writer io.Writer = os.Stdout
	if *output != "" {
		f, err := os.OpenFile(*output, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
		die(err)
		defer f.Close()
		writer = f
	}
	if *profile != "" {
		f, err := os.Create(*profile)
		die(err)
		die(pprof.StartCPUProfile(f))
		defer f.Close()
		defer pprof.StopCPUProfile()
	}
	ctx := d2log.With(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	enc := json.NewEncoder(writer)
	failures, ran := 0, 0
	for iteration := 0; iteration < *count; iteration++ {
		for _, tc := range cases {
			if tc.Skip || !match.MatchString(tc.Name) {
				continue
			}
			r := runCase(ctx, tc, iteration, font, *reroute, *artifacts)
			die(enc.Encode(r))
			ran++
			if r.Error != "" {
				failures++
			}
			fmt.Fprintf(os.Stderr, "%s iteration=%d tala=%.3fs error=%t\n", tc.Name, iteration, float64(r.LayoutNS+r.RoutingNS)/1e9, r.Error != "")
		}
	}
	if *memprofile != "" {
		f, err := os.Create(*memprofile)
		die(err)
		runtime.GC()
		die(pprof.WriteHeapProfile(f))
		die(f.Close())
	}
	fmt.Fprintf(os.Stderr, "completed=%d failures=%d gomaxprocs=%d go=%s corpus_sha256=%s\n", ran, failures, runtime.GOMAXPROCS(0), runtime.Version(), hash(b))
	// Return all records, including failures, before reporting an unsuccessful run.
	if failures != 0 || ran == 0 {
		// Deferred profile flushing must run before process exit.
		if *profile != "" {
			pprof.StopCPUProfile()
		}
		os.Exit(1)
	}
}
