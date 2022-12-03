package d2sequence

// leaves at least 25 units of space on the left/right when computing the space required between actors
const HORIZONTAL_PAD = 50.

// leaves at least 25 units of space on the top/bottom when computing the space required between messages
const VERTICAL_PAD = 50.

const MIN_ACTOR_DISTANCE = 250.

// min vertical distance between messages
const MIN_MESSAGE_DISTANCE = 80.

// default size
const SPAN_BASE_WIDTH = 12.

// as the spans start getting nested, their size grows
const SPAN_DEPTH_GROWTH_FACTOR = 8.

// when a span has a single messages
const MIN_SPAN_HEIGHT = 80.

const SPAN_MESSAGE_PAD = 16.

const LIFELINE_STROKE_WIDTH int = 2

const LIFELINE_STROKE_DASH int = 8
