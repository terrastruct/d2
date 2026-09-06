"""Build private, source-checked instrumentation without editing tracked files."""

import re
import subprocess
from pathlib import Path

HERE = Path(__file__).resolve().parent


def git(repo, *args):
    return subprocess.check_output(["git", "-C", str(repo), *args], text=True)


def replace(source, old, new, count=1):
    actual = source.count(old)
    if actual != count:
        raise ValueError(
            f"Instrumentation source changed: expected {count} occurrence(s), "
            f"found {actual}: {old!r}"
        )
    return source.replace(old, new)


def prepare(repo, output, base_ref):
    def read(path):
        if base_ref:
            return git(repo, "show", f"{base_ref}:{path}")
        return (repo / path).read_text()

    source = read("d2lib/d2.go")
    source = replace(source, '"context"', '''"context"
        "crypto/sha256"
        "encoding/json"
        "os"
        "path/filepath"
        "runtime/pprof"
        "sync"
        "time"''')
    source = replace(
        source, "\tg, config, err := d2compiler.Compile",
        '\tprobeCompiler := NonlayoutProbe(ctx, "compiler")\n'
        "\tg, config, err := d2compiler.Compile",
    )
    source = replace(source, "\t\tFS:       compileOpts.FS,\n\t})",
                     "\t\tFS:       compileOpts.FS,\n\t})\n probeCompiler()")

    # These statements are disjoint. The enclosing compile_total is reported
    # separately and is never added to its own component measurements.
    statements = [
        ("theme", "\terr := g.ApplyTheme(*renderOpts.ThemeID)"),
        ("dimensions", "\t\terr := g.SetDimensions(compileOpts.MeasuredTexts, "
         "compileOpts.Ruler, compileOpts.FontFamily, compileOpts.MonoFontFamily)"),
        ("layout_excluded", "\t\t\terr = d2layouts.LayoutNested(ctx, g, "
         "graphInfo, coreLayout, edgeRouter)"),
        ("export", "\td, err := d2exporter.Export(ctx, g, compileOpts.FontFamily, "
         "compileOpts.MonoFontFamily)"),
    ]
    for phase, statement in statements:
        variable = "probe_" + phase
        source = replace(source, statement,
                         f'\n{variable} := NonlayoutProbe(ctx, "{phase}")\n'
                         + statement + f"\n{variable}()")
    helper = (HERE / "probe.go.txt").read_text()
    source += "\n" + replace(helper, "package d2lib\n\n", "")

    test = read("e2etests/e2e_test.go")
    test = replace(test, "func TestE2E(t *testing.T) {",
                   "func TestE2E(t *testing.T) {\n"
                   "t.Cleanup(func() { assert.Success(t, d2lib.NonlayoutProbeWrite()) })")
    test = replace(test, "ctx = log.Leveled(ctx, slog.LevelDebug)",
                   "ctx = log.Leveled(ctx, slog.LevelError)\n"
                   'ctx = d2lib.NonlayoutCase(ctx, strings.TrimPrefix(t.Name(), "TestE2E/"))', 2)
    test = replace(test,
                   "func serde(t *testing.T, tc testCase, ruler *textmeasure.Ruler) {",
                   "func serde(t *testing.T, tc testCase, ruler *textmeasure.Ruler, "
                   "ctx context.Context) {\n"
                   'probeSerde := d2lib.NonlayoutProbe(ctx, "serde_compiler")')
    test = replace(test, "serde(t, tc, ruler)", "serde(t, tc, ruler, ctx)", 2)
    test = replace(test, "\t\tUTF16Pos: false,\n\t})",
                   "\t\tUTF16Pos: false,\n\t})\nprobeSerde()")

    statements = [
        ("serde_dimensions", "\t\terr = g.SetDimensions(nil, ruler, nil, nil)"),
        ("serde_serialize", "\tb, err := d2graph.SerializeGraph(g)"),
        ("serde_deserialize", "\terr = d2graph.DeserializeGraph(b, &newG)"),
        ("serde_serialize", "\troundTripBytes, err := d2graph.SerializeGraph(&newG)"),
        ("serde_json_compare", "\ttrequire.JSONEq(t, string(b), string(roundTripBytes))"),
        ("ruler", "\truler, err := textmeasure.NewRuler()"),
        ("ruler", "\t\truler, err = textmeasure.NewRuler()"),
    ]
    for index, (phase, statement) in enumerate(statements):
        variable = f"probe_{index}"
        test = replace(test, statement,
                       f'\n{variable} := d2lib.NonlayoutProbe(ctx, "{phase}")\n'
                       + statement + f"\n{variable}()")

    test = replace(test, "\tfor _, layoutName := range layoutsTested {",
                   "\tfor _, layoutName := range layoutsTested {\n"
                   'ctx = d2lib.NonlayoutCase(ctx, strings.TrimPrefix(t.Name(), "TestE2E/")+"/"+layoutName)')
    test = replace(test, "\tplugin := &d2plugin.ELKPlugin",
                   'ctx = d2lib.NonlayoutCase(ctx, strings.TrimPrefix(t.Name(), "TestE2E/")+"/elk")\n'
                   "\tplugin := &d2plugin.ELKPlugin")

    # Two runners execute the real active cases: ordinary E2E and ASCII txtar.
    # Newline prefixes prevent matching a statement with deeper indentation.
    for indent in ("\n\t", "\n\t\t"):
        statements = [
            ("compile_total", indent + "diagram, g, err := d2lib.Compile(ctx, "
             "tc.script, compileOpts, renderOpts)"),
            ("svg_render", indent + "boards, err := d2svg.RenderMultiboard(diagram, renderOpts)"),
            ("xml_verify", indent + "err = xml.Unmarshal(svgBytes, &xmlParsed)"),
            ("animation", indent + "\tsvgBytes, err = d2animate.Wrap(diagram, boards, *renderOpts, 1000)"),
        ]
        for phase, statement in statements:
            prefix = ""
            suffix = ""
            if phase == "svg_render":
                prefix = "\nassert.Success(t, d2lib.NonlayoutCapture(ctx, diagram, renderOpts))\n"
            elif phase == "xml_verify":
                suffix = '\nd2lib.NonlayoutFingerprint(ctx, "svg", svgBytes)'
            variable = "probe_" + phase
            test = replace(test, statement, prefix
                           + f'\n{variable} := d2lib.NonlayoutProbe(ctx, "{phase}")'
                           + statement + f"\n{variable}()" + suffix)

    for mode in ("extended", "standard"):
        statement = (f"\t{mode}Bytes, err := {mode}AsciiArtist.Render(ctx, "
                     f"diagram, {mode}RenderOpts)")
        test = replace(test, statement,
                       f'\nprobe_{mode} := d2lib.NonlayoutProbe(ctx, "ascii_{mode}")\n'
                       + statement + f"\nprobe_{mode}()\n"
                       + f'd2lib.NonlayoutFingerprint(ctx, "ascii_{mode}", {mode}Bytes)')

    output.mkdir(parents=True, exist_ok=True)
    (output / "d2.go").write_text(source)
    (output / "e2e_test.go").write_text(test)
    mapping = {
        str(repo / "d2lib/d2.go"): str(output / "d2.go"),
        str(repo / "e2etests/e2e_test.go"): str(output / "e2e_test.go"),
    }
    if base_ref:
        paths = set(git(repo, "diff", "--name-only", base_ref, "--", "*.go").splitlines())
        paths.update(git(repo, "ls-files", "--others", "--exclude-standard", "--", "*.go").splitlines())
        for index, path in enumerate(sorted(paths)):
            if str(repo / path) in mapping:
                continue
            replacement = output / f"base-{index}.go"
            exists = subprocess.run(
                ["git", "-C", str(repo), "cat-file", "-e", f"{base_ref}:{path}"],
                stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
            ).returncode == 0
            if exists:
                contents = read(path)
            else:
                # A file introduced after the baseline must not contribute code,
                # tests, or initializers to the baseline binary.
                current = (repo / path).read_text()
                match = re.search(r"(?m)^package\s+(\w+)", current)
                if not match:
                    raise ValueError(f"Cannot identify package in {path}")
                contents = f"package {match.group(1)}\n"
            replacement.write_text(contents)
            mapping[str(repo / path)] = str(replacement)
    subprocess.run(["gofmt", "-w", str(output / "d2.go"), str(output / "e2e_test.go")], check=True)
    return mapping
