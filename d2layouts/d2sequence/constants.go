package d2sequence

// leaves at least 25 units of space on the left/right when computing the space required between actors
const HORIZONTAL_PAD = 50.

// leaves at least 25 units of space on the top/bottom when computing the space required between edges
const VERTICAL_PAD = 50.

const MIN_ACTOR_DISTANCE = 200.

// min vertical distance between edges
const MIN_EDGE_DISTANCE = 100.

// default size
const ACTIVATION_BOX_WIDTH = 20.

// small pad so that edges don't touch lifelines and activation boxes
const ACTIVATION_BOX_EDGE_PAD = 2.

// as the activation boxes start getting nested, their size grows
const ACTIVATION_BOX_DEPTH_GROW_FACTOR = 10.

// when a activation box has a single edge
const DEFAULT_ACTIVATION_BOX_HEIGHT = MIN_EDGE_DISTANCE / 2.
