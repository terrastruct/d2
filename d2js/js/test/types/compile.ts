import {
  D2,
  type CompileOptions,
  type CompileRequest,
  type CompileResponse,
  type RenderOptions,
} from "../../index.js";

declare const d2: D2;
declare const input: string | CompileRequest;

const source = "x -> y";
const request: CompileRequest = { fs: { index: source } };
const options: CompileOptions = { layout: "tala", sketch: false };

const defaults: Promise<CompileResponse> = d2.compile(source);
const configured: Promise<CompileResponse> = d2.compile(source, options);
const imported: Promise<CompileResponse> = d2.compile(request);
const overridden: Promise<CompileResponse> = d2.compile(request, options);
const dynamic: Promise<CompileResponse> = d2.compile(input, options);

for (const layout of ["dagre", "elk", "tala"] as const) {
  d2.compile(source, { layout });
  d2.compile({ ...request, inputPath: "index", options: { layout } });
}

configured.then((response) => {
  const renderOptions: RenderOptions = response.renderOptions;
  d2.render(response.diagram, renderOptions);

  // @ts-expect-error The response exposes renderOptions, not options.
  response.options;
});

// @ts-expect-error Compile options belong directly in the second argument.
d2.compile(source, { options: { layout: "tala" } });
// @ts-expect-error Object input uses the same flat second-argument options.
d2.compile(request, { options: { layout: "tala" } });
// @ts-expect-error Unsupported layout names are rejected.
d2.compile(source, { layout: "unknown" });
// @ts-expect-error An object input must supply its file contents.
d2.compile({ inputPath: "index" });
// @ts-expect-error Layout options belong in input.options, not the request root.
d2.compile({ fs: { index: source }, layout: "tala" });
