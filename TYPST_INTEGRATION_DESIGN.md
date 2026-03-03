# Typst Integration Design Document

**Status**: Proposal  
**Author**: @dirk (GitHub issue #1435)  
**Date**: March 3, 2026  
**Target**: D2 v0.8.0

---

## Executive Summary

This document proposes adding **Typst support** to D2 following the established **renderer pattern** (`d2renderers/d2typst/`) rather than the plugin pattern. After evaluating three implementation approaches (CLI wrapper, Rust library via CGO, and WASM embedding), we recommend **Option 1: CLI Wrapper** for initial implementation, with **Option 3: WASM Embedding** as a future enhancement for browser environments.

---

## Background

### User Request
Issue #1435 requests Typst support in D2. Typst is a modern typesetting system with WASM capabilities, making it suitable for diagram label rendering.

### Architecture Decision: Renderer vs Plugin

D2 has two integration patterns:
- **Plugins** (`d2plugin/`) - for layout engines (dagre, elk)
- **Renderers** (`d2renderers/`) - for content rendering (LaTeX, SVG, etc.)

**Decision**: Typst should be a **renderer** (like LaTeX), not a plugin.

**Rationale**:
1. Typst renders diagram labels/text content, not layouts
2. LaTeX renderer (`d2renderers/d2latex/`) is the perfect precedent
3. Integration points are identical (label rendering in `d2graph/d2graph.go` and `d2renderers/d2svg/d2svg.go`)

---

## Implementation Approaches Evaluated

### Option 1: CLI Wrapper (RECOMMENDED)

**Description**: Shell out to `typst compile` CLI to convert typst markup → SVG

**Implementation**:
```go
// d2renderers/d2typst/typst_common.go
func Render(s string) (string, error) {
    cmd := exec.Command("typst", "compile", "-", "--format", "svg")
    cmd.Stdin = strings.NewReader(s)
    cmd.Stderr = &stderr
    output, err := cmd.Output()
    if err != nil {
        return "", parseTypstError(stderr.String())
    }
    return string(output), nil
}

func Measure(s string) (width, height int, error) {
    svg, err := Render(s)
    if err != nil {
        return 0, 0, err
    }
    // Parse SVG viewBox/dimensions
    return parseSVGDimensions(svg)
}
```

**Pros**:
- ✅ **Simple**: ~100 lines of Go code
- ✅ **Proven**: Existing `go-typst` wrapper demonstrates viability
- ✅ **Fast**: Sub-second compilation for typical labels
- ✅ **Cross-platform**: Typst CLI available for all platforms (Linux, macOS, Windows, ARM)
- ✅ **WASM-compatible**: Preserves D2's Go→wasm32 build (no CGO)
- ✅ **Maintainable**: Decoupled from Typst internals

**Cons**:
- ❌ **External dependency**: Requires `typst` binary in PATH or bundled
- ❌ **Process overhead**: ~1-5ms spawn cost per label (negligible for diagrams)
- ❌ **Not browser-native**: Cannot run in d2-playground without server

**Deployment Options**:
1. Document `typst` installation requirement (like LaTeX currently)
2. Bundle Typst CLI binary with D2 releases (Apache 2.0 license allows)
3. Auto-download Typst on first use (like TailwindCSS CLI pattern)

---

### Option 2: Rust Library via CGO (NOT RECOMMENDED)

**Description**: Embed Typst's Rust library via CGO/FFI

**Blocker**: **CGO and WASM are mutually exclusive**. D2 compiles to `GOOS=js GOARCH=wasm`, which requires `CGO_ENABLED=0`. CGO requires `CGO_ENABLED=1`.

**Evidence**: 
- [Go WASM docs](https://github.com/golang/go/wiki/WebAssembly): "CGO is not supported when compiling to WASM"
- D2's existing WASM build (`d2js/`) would break immediately

**Additional Issues**:
- ❌ Cross-compilation nightmares (requires C/Rust toolchain for each target)
- ❌ Binary bloat (can't use `scratch`/`distroless` Docker images)
- ❌ libc/musl version conflicts
- ❌ Complex build infrastructure

**Verdict**: **RULED OUT** due to WASM incompatibility.

---

### Option 3: WASM Embedding (FUTURE ENHANCEMENT)

**Description**: Embed Typst's WASM build (typst.ts) using D2's existing JavaScript runner

**Implementation** (mirrors LaTeX pattern):
```go
// d2renderers/d2typst/typst_embed.go
//go:build !js || !wasm

//go:embed typst-compiler.wasm.br
var typstWASMBr []byte

var typstWASM string

func init() {
    var err error
    typstWASM, err = compression.DecompressBrotli(typstWASMBr)
    if err != nil {
        panic(fmt.Sprintf("Failed to decompress Typst WASM: %v", err))
    }
}

// d2renderers/d2typst/typst_common.go
func Render(s string) (string, error) {
    runner := jsrunner.NewJSRunner()
    
    // Load Typst WASM
    if _, err := runner.RunString(typstWASM); err != nil {
        return "", err
    }
    
    // Compile typst → SVG
    val, err := runner.RunString(fmt.Sprintf(`
        $typst.svg({ mainContent: %s })
    `, strconv.Quote(s)))
    if err != nil {
        return "", err
    }
    
    return val.String(), nil
}
```

**Pros**:
- ✅ **Self-contained**: No external binary required
- ✅ **Browser-compatible**: Works in d2-playground (client-side rendering)
- ✅ **Mirrors LaTeX pattern**: Same architecture as existing `d2latex/` renderer
- ✅ **Bundle size manageable**: Renderer-only WASM is 1.1 MB (vs MathJax 20 MB)

**Cons**:
- ❌ **Large compiler WASM**: Full compiler is 28.5 MB (mitigated by using renderer-only mode)
- ❌ **JavaScript runner dependency**: Requires V8go (Goja has limited WASM support)
- ❌ **More complex**: ~300 lines vs ~100 for CLI wrapper

**Verdict**: **VIABLE** but defer to Phase 2 after CLI implementation proven.

---

## Recommended Implementation Plan

### Phase 1: CLI Wrapper (Target: v0.8.0)

**Week 1-2: Core Implementation**
1. Create `d2renderers/d2typst/` package
2. Implement `Render()` and `Measure()` following LaTeX pattern
3. Shell out to `typst compile --format svg`
4. Parse SVG output for dimensions (regex on `viewBox` attribute)

**Week 2-3: Integration**
1. Add language detection in `d2graph/d2graph.go`:
   ```go
   if obj.Language == "typst" {
       width, height, err := d2typst.Measure(obj.Text().Text)
   }
   ```
2. Add rendering in `d2renderers/d2svg/d2svg.go`:
   ```go
   if targetShape.Language == "typst" {
       render, err := d2typst.Render(targetShape.Label)
   }
   ```

**Week 3-4: Testing**
1. Unit tests (`typst_test.go`)
2. E2E tests in `e2etests/testdata/stable/typst_*/`
3. Test with both dagre and elk layouts
4. Visual regression tests via `./ci/e2ereport.sh`

**Week 4: PR Submission**
1. Update `ci/release/changelogs/next.md`
2. Create screenshot for PR description
3. Submit PR referencing issue #1435

### Phase 2: WASM Enhancement (Target: v0.9.0)

**Post Phase 1 completion**:
1. Evaluate JavaScript runner (confirm V8go usage)
2. Embed typst.ts WASM (renderer-only, 1.1 MB)
3. Enable client-side rendering in d2-playground
4. Benchmark performance vs CLI approach

---

## Integration Points (Following LaTeX Pattern)

### 1. Dimension Calculation (`d2graph/d2graph.go`)

**Current LaTeX pattern** (lines 957-962):
```go
if obj.Language == "latex" {
    width, height, err := d2latex.Measure(obj.Text().Text)
    if err != nil {
        return nil, err
    }
    dims = d2target.NewTextDimensions(width, height)
}
```

**Typst addition**:
```go
if obj.Language == "typst" {
    width, height, err := d2typst.Measure(obj.Text().Text)
    if err != nil {
        return nil, err
    }
    dims = d2target.NewTextDimensions(width, height)
}
```

### 2. SVG Rendering (`d2renderers/d2svg/d2svg.go`)

**Current LaTeX pattern** (lines 2011-2020):
```go
if targetShape.Language == "latex" {
    render, err := d2latex.Render(targetShape.Label)
    if err != nil {
        return labelMask, err
    }
    // Strip XML declaration/DOCTYPE
    render = strings.ReplaceAll(render, xmlDecl, "")
    render = strings.ReplaceAll(render, doctype, "")
    // Embed in <g> element
    gEl := d2themes.NewThemableElement("g", inlineTheme)
    gEl.Content = render
}
```

**Typst addition** (identical pattern):
```go
if targetShape.Language == "typst" {
    render, err := d2typst.Render(targetShape.Label)
    if err != nil {
        return labelMask, err
    }
    // Strip XML declaration if present
    render = cleanSVGOutput(render)
    gEl := d2themes.NewThemableElement("g", inlineTheme)
    gEl.Content = render
}
```

---

## File Structure

```
d2renderers/d2typst/
├── typst_common.go          # Core Render() and Measure() (Phase 1)
├── typst_cli.go             # CLI wrapper implementation (Phase 1)
├── typst_embed.go           # WASM embedding (Phase 2)
├── typst_embed_wasm.go      # Build tags for WASM (Phase 2)
├── typst_test.go            # Unit tests
└── typst-compiler.wasm.br   # Compressed WASM (Phase 2)
```

---

## Testing Strategy

### Unit Tests (`typst_test.go`)

```go
func TestRender(t *testing.T) {
    txts := []string{
        `$ a + b = c $`,
        `#rect[Hello, Typst!]`,
        `#table(
            columns: 2,
            [A], [B],
            [C], [D],
        )`,
    }
    for _, txt := range txts {
        svg, err := Render(txt)
        if err != nil {
            t.Fatal(err)
        }
        var xmlParsed interface{}
        if err := xml.Unmarshal([]byte(svg), &xmlParsed); err != nil {
            t.Fatalf("invalid SVG: %v", err)
        }
    }
}

func TestMeasure(t *testing.T) {
    width, height, err := Measure(`$ x^2 + y^2 = r^2 $`)
    if err != nil {
        t.Fatal(err)
    }
    if width <= 0 || height <= 0 {
        t.Fatalf("invalid dimensions: %dx%d", width, height)
    }
}
```

### E2E Tests

Create test cases in `e2etests/testdata/stable/typst_*/`:
- `typst_basic.d2` - Simple label rendering
- `typst_math.d2` - Mathematical equations
- `typst_table.d2` - Structured content
- `typst_multiline.d2` - Multi-line text blocks

Run with:
```bash
TESTDATA_ACCEPT=1 go test ./e2etests -run TestTypst
./ci/e2ereport.sh -delta
```

---

## Error Handling

### Typst CLI Errors

Typst outputs structured errors to stderr:

```
error: unknown variable
  ┌─ input.typ:1:1
  │
1 │ #unknownVar
  │  ^^^^^^^^^^^
  │
  = hint: if you meant to use a string, try adding quotes
```

**Parse errors** and return meaningful Go errors:
```go
func parseTypstError(stderr string) error {
    // Extract line number, error message
    // Return user-friendly error
    return fmt.Errorf("typst compilation failed: %s", cleanedError)
}
```

### Graceful Degradation

If `typst` binary not found:
```go
if errors.Is(err, exec.ErrNotFound) {
    return "", fmt.Errorf("typst binary not found in PATH. Install from https://typst.app/docs/")
}
```

---

## Documentation Requirements

### 1. Language Docs PR

Submit PR to [d2-docs](https://github.com/terrastruct/d2-docs) adding Typst section:

```markdown
## Typst

D2 supports Typst for advanced label rendering.

\`\`\`d2
x: |typst
  $ sum_(i=1)^n i = (n(n+1))/2 $
|
\`\`\`

### Installation

Typst support requires the Typst CLI:

- macOS: `brew install typst`
- Linux: Download from [releases](https://github.com/typst/typst/releases)
- Windows: `winget install Typst.Typst`
```

### 2. Changelog Entry

Add to `ci/release/changelogs/next.md`:

```markdown
#### Features

- Typst renderer support (#1435) - Add `language: typst` for diagram labels using Typst markup
```

### 3. README Update

Add to "Related" section under community plugins (if applicable).

---

## Performance Considerations

### Benchmarks (Expected)

| Operation | LaTeX (MathJax) | Typst CLI | Typst WASM |
|-----------|----------------|-----------|------------|
| Cold start | ~50ms | ~5ms | ~100ms (WASM init) |
| Single label | ~10ms | ~2ms | ~5ms |
| 100 labels | ~200ms | ~150ms | ~200ms |

**Note**: Actual benchmarks to be measured during implementation.

### Optimization Strategies

1. **Caching**: Cache rendered SVGs by content hash (future enhancement)
2. **Batching**: If multiple labels exist, consider batching compilation (future)
3. **Lazy loading**: Only compile labels when rendering, not during parsing

---

## Security Considerations

### Typst Sandboxing

Typst CLI has built-in sandboxing:
- No network access during compilation
- File system access restricted to `--root` directory
- Safe for untrusted input markup

**D2 should set `--root`** to prevent access outside project directory:
```go
cmd := exec.Command("typst", "compile", 
    "--root", projectRoot,  // Restrict file access
    "-", "--format", "svg")
```

### SVG Output Sanitization

Typst-generated SVGs are safe (no scripts), but validate:
- No `<script>` tags
- No event handlers (`onclick`, etc.)
- Valid XML structure

---

## Migration Path for Users

### Existing Diagrams

No breaking changes - existing diagrams continue to work.

### New Feature Opt-in

```d2
# Before (plain text)
database: My Database

# After (typst rendering)
database: |typst
  *My Database*
  `Version 2.0`
|

# Or inline syntax (if implemented)
database.label: "typst:*Emphasized*"
```

---

## Alternatives Considered

### Alternative 1: Fork Typst as Go Library

**Rationale**: Rewrite Typst in Go to avoid external dependency

**Rejected**: 
- Massive engineering effort (Typst is 50k+ lines of Rust)
- Maintenance burden (track upstream changes)
- Lower quality (Typst team's expertise in typesetting)

### Alternative 2: Markdown with Math Extensions

**Rationale**: Use existing markdown renderer with KaTeX/MathJax

**Rejected**:
- D2 already has markdown support
- Typst offers richer typesetting (tables, layouts, styling)
- User request specifically asked for Typst

### Alternative 3: Wait for Official D2 Typst Plugin

**Rationale**: Let D2 maintainers implement if/when they want

**Rejected**:
- Issue #1435 labeled "good first issue" - community contribution welcome
- No indication of maintainer implementation timeline
- We have capacity to implement now

---

## Open Questions

1. **Should we support inline typst syntax?** (e.g., `label: "typst:$ x^2 $"`)
   - **Decision**: Start with language blocks only, evaluate inline syntax in Phase 2

2. **How to handle multi-page Typst output?**
   - **Decision**: Only render first page for labels (consistent with LaTeX behavior)

3. **Should we bundle Typst CLI or require external install?**
   - **Decision**: Document external install initially, consider bundling if user feedback requests it

4. **Font handling for Typst?**
   - **Decision**: Use Typst's default fonts initially, add `--font-path` support if requested

---

## Success Metrics

### Phase 1 Completion Criteria

- [ ] `d2renderers/d2typst/` package implemented
- [ ] `Render()` and `Measure()` functions working
- [ ] Unit tests passing
- [ ] E2E tests passing with visual regression
- [ ] Documentation updated (language docs, changelog)
- [ ] PR merged to D2 main repository

### User Adoption (Post-Release)

- Usage in community diagrams (track via GitHub search)
- Feature requests for enhancements
- Bug reports (aim for <5 issues in first month)

---

## Risks & Mitigation

| Risk | Impact | Likelihood | Mitigation |
|------|--------|-----------|------------|
| Typst CLI not available on platform | High | Low | Document installation, provide pre-built binaries |
| Performance too slow | Medium | Low | Benchmark early, optimize, or pivot to WASM |
| Maintainer rejects approach | High | Low | Align with contribution guidelines, get early feedback |
| SVG dimension parsing fragile | Medium | Medium | Use robust XML parser, handle edge cases |
| Breaking changes in Typst CLI | Medium | Low | Pin to stable Typst version in tests, document requirements |

---

## Timeline Summary

**Phase 1 (CLI Wrapper)**: 4 weeks
- Week 1: Core implementation
- Week 2: Integration
- Week 3: Testing
- Week 4: PR submission

**Phase 2 (WASM Enhancement)**: 2-3 weeks (post Phase 1)
- Week 1: WASM embedding
- Week 2: Testing & optimization

**Total**: 6-7 weeks to full feature parity with LaTeX renderer

---

## References

- Issue #1435: https://github.com/terrastruct/d2/issues/1435
- Typst Documentation: https://typst.app/docs/
- D2 LaTeX Renderer: `d2renderers/d2latex/`
- Typst.ts (WASM): https://github.com/Myriad-Dreamin/typst.ts
- go-typst wrapper: https://github.com/Dadido3/go-typst
- D2 Contributing Guide: https://github.com/terrastruct/d2/blob/main/CONTRIBUTING.md

---

## Approval & Sign-off

**Proposed by**: @dirk  
**Reviewed by**: (pending maintainer review)  
**Approved by**: (pending)  
**Implementation start**: (pending approval)
