declare module "@terrastruct/d2" {
  interface Options {
    /**
     * @default 0
     * Set the diagram theme ID.
     */
    theme?: number;
    /**
     * @default -1
     * The theme to use when the viewer's browser is in dark mode.
     * When left unset --theme is used for both light and dark mode.
     * Be aware that explicit styles set in D2 code will still be
     * applied and this may produce unexpected results. We plan on
     * resolving this by making style maps in D2 light/dark mode
     * specific. See https://github.com/terrastruct/d2/issues/831.
     */
    darkTheme?: number;
    /**
     * Set the diagram layout engine to the passed string. For a
     * list of available options, run layout.
     */
    layout?: "elk" | "dagre";
    /**
     * @default 100
     * Pixels padded around the rendered diagram.
     */
    pad?: number;
    /**
     * @default -1
     * Scale the output. E.g., 0.5 to halve the default size.
     * Default -1 means that SVG's will fit to screen and all others
     * will use their default render size. Setting to 1 turns off
     * SVG fitting to screen.
     */
    scale?: number;
    /**
     * @default false
     * Renders the diagram to look like it was sketched by hand.
     */
    sketch?: boolean;
    /**
     * @default true
     * Bundle all assets and layers into the output svg.
     */
    bundle?: boolean;
    /**
     * Center the SVG in the containing viewbox, such as your
     * browser screen.
     */
    center?: boolean;
  }

  export interface CompileRequest {
    fs?: {
      index: string;
    };
    options: Options;
  }

  export interface RenderResult {
    svg: string;
  }

  export interface EncodedResult {
    encoded: string;
  }

  export interface DecodedResult {
    decoded: string;
  }

  export type WorkerMessage =
    | { type: "ready" }
    | { type: "error"; error: string }
    | {
        type: "result";
        data: string | EncodedResult | DecodedResult;
      };

  export interface CompileResult {
    result: string;
  }

  export interface D2Worker {
    on(event: "message", listener: (data: WorkerMessage) => void): void;
    on(event: "error", listener: (error: Error) => void): void;
    onmessage?: (e: { data: WorkerMessage }) => void;
    onerror?: (error: Error) => void;
    postMessage(message: { type: string; data: object }): void;
  }

  export class D2 {
    readonly ready: Promise<void>;
    worker: D2Worker;
    currentResolve?: (
      result: string | RenderResult | EncodedResult | DecodedResult
    ) => void;
    currentReject?: (reason: Error) => void;

    constructor();

    /**
     * Sets up the message handler for the worker.
     */
    setupMessageHandler(): Promise<void>;

    /**
     * Initializes the worker and related resources.
     */
    init(): Promise<void>;

    /**
     * Sends a message to the worker.
     * @param type The type of message.
     * @param data The message payload.
     */
    sendMessage(
      type: string,
      data: object
    ): Promise<CompileResult | RenderResult | EncodedResult | DecodedResult>;

    /**
     * Compiles the provided input.
     * @param input A string representing the source or a CompileRequest.
     * @param options Optional compilation options.
     */
    compile(input: string | CompileRequest, options?: Options): Promise<string>;

    /**
     * Renders the given diagram.
     * @param diagram A diagram definition in string form.
     * @param options Optional rendering options.
     */
    render(diagram: string, options?: Options): Promise<string>;

    /**
     * Encodes the provided script.
     * @param script The script to encode.
     */
    encode(script: string): Promise<EncodedResult>;

    /**
     * Decodes the provided encoded string.
     * @param encoded The encoded data.
     */
    decode(encoded: string): Promise<DecodedResult>;
  }
}
