declare module "@terrastruct/d2" {
  interface RenderOptions {
    /** Enable sketch mode [default: false] */
    sketch?: boolean;
    /** Theme ID to use [default: 0] */
    themeID?: number;
    /** Theme ID to use when client is in dark mode */
    darkThemeID?: number;
    /** Center the SVG in the containing viewbox [default: false] */
    center?: boolean;
    /** Pixels padded around the rendered diagram [default: 100] */
    pad?: number;
    /** Scale the output. E.g., 0.5 to halve the default size. The default will render SVG's that will fit to screen. Setting to 1 turns off SVG fitting to screen. */
    scale?: number;
    /** Adds an appendix for tooltips and links [default: false] */
    forceAppendix?: boolean;
    /** Target board/s to render. If target ends with '', it will be rendered with all of its scenarios, steps, and layers. Otherwise, only the target board will be rendered. E.g. target: 'layers.x.*' to render layer 'x' with all of its children. Pass '' to render all scenarios, steps, and layers. By default, only the root board is rendered. Multi-board outputs are currently only supported for animated SVGs and so animateInterval must be set to a value greater than 0 when targeting multiple boards. */
    target?: string;
    /** If given, multiple boards are packaged as 1 SVG which transitions through each board at the interval (in milliseconds). */
    animateInterval?: number;
    /** Add a salt value to ensure the output uses unique IDs. This is useful when generating multiple identical diagrams to be included in the same HTML doc, so that duplicate IDs do not cause invalid HTML. The salt value is a string that will be appended to IDs in the output. */
    salt?: string;
    /** Omit XML tag (<?xml ...?>) from output SVG files. Useful when generating SVGs for direct HTML embedding. */
    noXMLTag?: boolean;
  }

  interface CompileOptions extends RenderOptions {
    /** Layout engine to use [default: 'dagre'] */
    layout?: 'dagre' | 'elk';
    /** A byte array containing .ttf file to use for the regular font. If none provided, Source Sans Pro Regular is used. */
    fontRegular?: Uint8Array;
    /** A byte array containing .ttf file to use for the italic font. If none provided, Source Sans Pro Italic is used. */
    fontItalic?: Uint8Array;
    /** A byte array containing .ttf file to use for the bold font. If none provided, Source Sans Pro Bold is used. */
    fontBold?: Uint8Array;
    /** A byte array containing .ttf file to use for the semibold font. If none provided, Source Sans Pro Semibold is used. */
    fontSemibold?: Uint8Array;
  }

  interface CompileRequest {
    /** A mapping of D2 file paths to their content*/
    fs: Record<string, string>;
    /** The path of the entry D2 file [default: index]*/
    inputPath: string;
    /** The CompileOptions to pass to the compiler*/
    options: CompileOptions;
  }

  interface Diagram {
    config: RenderOptions;
  }

  interface CompileResult {
    /**  Compiled D2 diagram*/
    diagram: Diagram;
    /** RenderOptions: Render options merged with configuration set in diagram*/
    renderOptions: RenderOptions;
    fs: Record<string, string>;
    graph: unknown;
  }

  class D2 {
    compile(input: string | CompileRequest, options?: CompileOptions): Promise<CompileResult>;
    render(diagram: Diagram, options?: RenderOptions): Promise<string>;
  }
}
