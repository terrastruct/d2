declare module "@terrastruct/d2" {
  export interface Options {
    /**
     * @default "dagre"
     * Set the diagram layout engine.
     */
    layout?: "elk" | "dagre";

    /**
     * @default false
     * Renders the diagram to look like it was sketched by hand.
     */
    sketch?: boolean;

    /**
     * @default 0
     * Set the diagram theme ID.
     */
    themeId?: number;

    /**
     * @default -1
     * The theme to use when the viewer's browser is in dark mode.
     */
    darkTheme?: number;

    /**
     * @default 100
     * Pixels padded around the rendered diagram.
     */
    pad?: number;

    /**
     * @default -1
     * Scale the output. E.g., 0.5 to halve the default size.
     */
    scale?: number;

    /**
     * @default true
     * Bundle all assets and layers into the output svg.
     */
    bundle?: boolean;

    /**
     * Center the SVG in the containing viewbox.
     */
    center?: boolean;
  }

  export interface CompileRequest {
    fs?: Record<string, string>;
    options?: Partial<Options>;
    [key: string]: unknown;
  }

  export interface CompileResult {
    result: string;
  }

  export interface RenderOptions {
    diagram: string;
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

  export interface InitData {
    wasm: ArrayBuffer | string;
    wasmExecContent: string | null;
    elkContent: string | null;
    wasmExecUrl: string | null;
  }

  export type MessageType = "init" | "compile" | "render" | "encode" | "decode";

  export type WorkerMessage =
    | { type: "ready" }
    | { type: "error"; error: string }
    | {
      type: "result";
      data:
        | string
        | CompileResult
        | RenderResult
        | EncodedResult
        | DecodedResult;
    };

  export interface WorkerInterface {
    on(event: "message", listener: (data: WorkerMessage) => void): void;
    on(event: "error", listener: (error: Error) => void): void;
    onmessage?: (e: { data: WorkerMessage }) => void;
    onerror?: (error: { message?: string }) => void;
    postMessage(message: { type: MessageType; data: unknown }): void;
  }

  export class D2 {
    readonly ready: Promise<void>;
    private worker: WorkerInterface;
    private currentResolve?: (
      result:
        | string
        | CompileResult
        | RenderResult
        | EncodedResult
        | DecodedResult,
    ) => void;
    private currentReject?: (reason: Error) => void;

    constructor();

    /**
     * Sets up the message handler for the worker.
     * @returns A promise that resolves when the worker is ready.
     */
    private setupMessageHandler(): Promise<void>;

    /**
     * Initializes the worker and related resources.
     * @returns A promise that resolves when initialization is complete.
     */
    private init(): Promise<void>;

    /**
     * Sends a message to the worker.
     * @param type The type of message.
     * @param data The message payload.
     * @returns A promise that resolves with the response.
     */
    private sendMessage<
      T extends
        | string
        | CompileResult
        | RenderResult
        | EncodedResult
        | DecodedResult,
    >(
      type: MessageType,
      data: unknown,
    ): Promise<T>;

    /**
     * Compiles the provided input.
     * @param input A string representing the source or a CompileRequest.
     * @param options Optional compilation options.
     * @returns A promise that resolves with the compiled result.
     */
    compile(input: string, options?: Partial<Options>): Promise<CompileResult>;
    compile(input: CompileRequest): Promise<CompileResult>;

    /**
     * Renders the given diagram.
     * @param diagram A diagram definition in string form.
     * @param options Optional rendering options.
     * @returns A promise that resolves with the rendered SVG.
     */
    render(diagram: string, options?: Partial<Options>): Promise<RenderResult>;

    /**
     * Encodes the provided script.
     * @param script The script to encode.
     * @returns A promise that resolves with the encoded result.
     */
    encode(script: string): Promise<EncodedResult>;

    /**
     * Decodes the provided encoded string.
     * @param encoded The encoded data.
     * @returns A promise that resolves with the decoded result.
     */
    decode(encoded: string): Promise<DecodedResult>;
  }
}
