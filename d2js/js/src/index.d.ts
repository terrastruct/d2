declare module "index" {
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

    interface Request {
        fs?: { index: string };
        options?: Options;
    }

    export class D2 {
        ready: Promise<void>;
        currentResolve?: (value: any) => void;
        currentReject?: (reason?: any) => void;
        worker: any;

        constructor();

        setupMessageHandler(): Promise<void>;

        init(): Promise<void>;

        sendMessage(type: string, data: any): Promise<any>;

        compile(input: string | Request, options?: Options): Promise<any>;

        render(diagram: string, options?: Options): Promise<any>;

        encode(script: string): Promise<any>;

        decode(encoded: string): Promise<any>;
    }
}
