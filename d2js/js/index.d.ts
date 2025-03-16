export class D2 {
  compile(input: string, options?: Omit<CompileRequest, "fs">): Promise<CompileResponse>;
  compile(input: CompileRequest): Promise<CompileResponse>;

  render(diagram: Diagram, options?: RenderOptions): Promise<string>;
}

export interface RenderOptions {
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

export interface CompileOptions extends RenderOptions {
  /** Layout engine to use [default: 'dagre'] */
  layout?: "dagre" | "elk";
  /** A byte array containing .ttf file to use for the regular font. If none provided, Source Sans Pro Regular is used. */
  fontRegular?: Uint8Array;
  /** A byte array containing .ttf file to use for the italic font. If none provided, Source Sans Pro Italic is used. */
  fontItalic?: Uint8Array;
  /** A byte array containing .ttf file to use for the bold font. If none provided, Source Sans Pro Bold is used. */
  fontBold?: Uint8Array;
  /** A byte array containing .ttf file to use for the semibold font. If none provided, Source Sans Pro Semibold is used. */
  fontSemibold?: Uint8Array;
}

export interface CompileRequest {
  /** A mapping of D2 file paths to their content*/
  fs: Record<string, string>;
  /** The path of the entry D2 file [default: index]*/
  inputPath: string;
  /** The CompileOptions to pass to the compiler*/
  options: CompileOptions;
}

export interface CompileResponse {
  /**  Compiled D2 diagram*/
  diagram: Diagram /* d2target.Diagram */;
  /** RenderOptions: Render options merged with configuration set in diagram*/
  renderOptions: RenderOptions;
  fs: Record<string, string>;
  graph: Graph;
  inputPath: string;
}

export interface Diagram {
  config?: RenderOptions;
  name: string;
  /**
   * See docs on the same field in d2graph to understand what it means.
   */
  isFolderOnly: boolean;
  description?: string;
  fontFamily?: any /* d2fonts.FontFamily */;
  shapes: Shape[];
  connections: Connection[];
  root: Shape;
  legend?: Legend;
  layers?: (Diagram | undefined)[];
  scenarios?: (Diagram | undefined)[];
  steps?: (Diagram | undefined)[];
}

export interface Legend {
  shapes?: Shape[];
  connections?: Connection[];
}

export type Shape = (Class | SQLTable | Text) & ShapeBase;

export interface ShapeBase {
  id: string;
  type: string;
  classes?: string[];
  pos: Point;
  width: number /* int */;
  height: number /* int */;
  opacity: number /* float64 */;
  strokeDash: number /* float64 */;
  strokeWidth: number /* int */;
  borderRadius: number /* int */;
  fill: string;
  fillPattern?: string;
  stroke: string;
  animated: boolean;
  shadow: boolean;
  "3d": boolean;
  multiple: boolean;
  "double-border": boolean;
  tooltip: string;
  link: string;
  prettyLink?: string;
  icon?: string /* url.URL */;
  iconPosition: string;
  /**
   * Whether the shape should allow shapes behind it to bleed through
   * Currently just used for sequence diagram groups
   */
  blend: boolean;
  contentAspectRatio?: number /* float64 */;
  labelPosition?: string;
  zIndex: number /* int */;
  level: number /* int */;
  /**
   * These are used for special shapes, sql_table and class
   */
  primaryAccentColor?: string;
  secondaryAccentColor?: string;
  neutralAccentColor?: string;
}

export interface Point {
  x: number /* int */;
  y: number /* int */;
}

export interface Class {
  fields: ClassField[];
  methods: ClassMethod[];
}

export interface ClassField {
  name: string;
  type: string;
  visibility: string;
}

export interface ClassMethod {
  name: string;
  return: string;
  visibility: string;
}

export interface SQLTable {
  columns: SQLColumn[];
}

export interface SQLColumn {
  name: Text;
  type: Text;
  constraint: string[];
  reference: string;
}

export interface Text {
  label: string;
  fontSize: number /* int */;
  fontFamily: string;
  language: string;
  color: string;
  italic: boolean;
  bold: boolean;
  underline: boolean;
  labelWidth: number /* int */;
  labelHeight: number /* int */;
  labelFill?: string;
}

export interface Connection extends Text {
  id: string;
  classes?: string[];
  src: string;
  srcArrow: Arrowhead;
  srcLabel?: Text;
  dst: string;
  dstArrow: Arrowhead;
  dstLabel?: Text;
  opacity: number /* float64 */;
  strokeDash: number /* float64 */;
  strokeWidth: number /* int */;
  stroke: string;
  fill?: string;
  borderRadius?: number /* float64 */;
  labelPosition: string;
  labelPercentage: number /* float64 */;
  link: string;
  prettyLink?: string;
  route: (any /* geo.Point */ | undefined)[];
  isCurve?: boolean;
  animated: boolean;
  tooltip: string;
  icon?: string /* url.URL */;
  iconPosition?: string;
  zIndex: number /* int */;
}

export type Arrowhead =
  | "none"
  | "arrow"
  | "unfilled-triangle"
  | "triangle"
  | "diamond"
  | "filled-diamond"
  | "circle"
  | "filled-circle"
  | "box"
  | "filled-box"
  | "line"
  | "cf-one"
  | "cf-many"
  | "cf-one-required"
  | "cf-many-required";

export interface Graph {
  name: string;
  /**
   * IsFolderOnly indicates a board or scenario itself makes no modifications from its
   * base. Folder only boards do not have a render and are used purely for organizing
   * the board tree.
   */
  isFolderOnly: boolean;
  ast?: any /* d2ast.Map */;
  root?: Object;
  legend?: Legend;
  edges: (Edge | undefined)[];
  objects: (Object | undefined)[];
  layers?: (Graph | undefined)[];
  scenarios?: (Graph | undefined)[];
  steps?: (Graph | undefined)[];
  theme?: any /* d2themes.Theme */;
  /**
   * Object.Level uses the location of a nested graph
   */
  rootLevel?: number /* int */;
  /**
   * Currently this holds data embedded from source code configuration variables
   * Plugins only have access to exported graph, so this data structure allows
   * carrying arbitrary metadata that any plugin might handle
   */
  data?: { [key: string]: any };
}

export interface Edge {
  index: number /* int */;
  srcTableColumnIndex?: number /* int */;
  dstTableColumnIndex?: number /* int */;
  labelPosition?: string;
  labelPercentage?: number /* float64 */;
  isCurve: boolean;
  route?: (any /* geo.Point */ | undefined)[];
  src_arrow: boolean;
  srcArrowhead?: Attributes;
  /**
   * TODO alixander (Mon Sep 12 2022): deprecate SrcArrow and DstArrow and just use SrcArrowhead and DstArrowhead
   */
  dst_arrow: boolean;
  dstArrowhead?: Attributes;
  references?: EdgeReference[];
  attributes?: Attributes;
  zIndex: number /* int */;
}

export interface Attributes {
  label: Scalar;
  labelDimensions: TextDimensions;
  style: Style;
  icon?: string /* url.URL */;
  tooltip?: Scalar;
  link?: Scalar;
  width?: Scalar;
  height?: Scalar;
  top?: Scalar;
  left?: Scalar;
  /**
   * TODO consider separate Attributes struct for shape-specific and edge-specific
   * Shapes only
   */
  near_key?: any /* d2ast.KeyPath */;
  language?: string;
  /**
   * TODO: default to ShapeRectangle instead of empty string
   */
  shape: Scalar;
  direction: Scalar;
  constraint: string[];
  gridRows?: Scalar;
  gridColumns?: Scalar;
  gridGap?: Scalar;
  verticalGap?: Scalar;
  horizontalGap?: Scalar;
  labelPosition?: Scalar;
  iconPosition?: Scalar;
  /**
   * These names are attached to the rendered elements in SVG
   * so that users can target them however they like outside of D2
   */
  classes?: string[];
}

export interface EdgeReference {
  map_key_edge_index: number /* int */;
}

export interface Scalar {
  value: string;
}

export interface Style {
  opacity?: Scalar;
  stroke?: Scalar;
  fill?: Scalar;
  fillPattern?: Scalar;
  strokeWidth?: Scalar;
  strokeDash?: Scalar;
  borderRadius?: Scalar;
  shadow?: Scalar;
  "3d"?: Scalar;
  multiple?: Scalar;
  font?: Scalar;
  fontSize?: Scalar;
  fontColor?: Scalar;
  animated?: Scalar;
  bold?: Scalar;
  italic?: Scalar;
  underline?: Scalar;
  filled?: Scalar;
  doubleBorder?: Scalar;
  textTransform?: Scalar;
}

export interface TextDimensions {
  width: number /* int */;
  height: number /* int */;
}
