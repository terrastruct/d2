package d2sequence

const HORIZONTAL_PAD = 40.

const VERTICAL_PAD = 20.

const MIN_ACTOR_DISTANCE = 150.

const MIN_ACTOR_WIDTH = 100.

const SELF_MESSAGE_HORIZONTAL_TRAVEL = 74

const GROUP_CONTAINER_PADDING = 24.

const GROUP_LABEL_PADDING = 20

// min vertical distance between messages
const MIN_MESSAGE_DISTANCE = 40.

// default size
const SPAN_BASE_WIDTH = 12.

// as the spans start getting nested, their size grows
const SPAN_DEPTH_GROWTH_FACTOR = 8.

// when a span has a single messages
const MIN_SPAN_HEIGHT = 60.

const SPAN_MESSAGE_PAD = 16.

const LIFELINE_STROKE_WIDTH int = 2

const LIFELINE_STROKE_DASH int = 4

// pad when the actor has the label placed OutsideMiddleBottom so that the lifeline is not so close to the text
const LIFELINE_LABEL_PAD = 5.

const (
	LIFELINE_Z_INDEX = 1
	SPAN_Z_INDEX     = 2
	GROUP_Z_INDEX    = 3
	MESSAGE_Z_INDEX  = 4
	NOTE_Z_INDEX     = 5
)
