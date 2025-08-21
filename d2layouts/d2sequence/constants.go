package d2sequence

// units of space on the left/right when computing the space required between actors
const HORIZONTAL_PAD = 40.
const LABEL_HORIZONTAL_PAD = 60.

// units of space on the top/bottom when computing the space required between messages
// TODO lower
const VERTICAL_PAD = 40.

const MIN_ACTOR_DISTANCE = 150.

const MIN_ACTOR_WIDTH = 100.

// ASCII-specific constants for minimal spacing
const ASCII_HORIZONTAL_PAD = 1.
const ASCII_LABEL_HORIZONTAL_PAD = 2.
const ASCII_VERTICAL_PAD = 1.
const ASCII_MIN_ACTOR_DISTANCE = 10.
const ASCII_MIN_ACTOR_WIDTH = 7.  // Minimum width for ASCII actors

const SELF_MESSAGE_HORIZONTAL_TRAVEL = 80.

const GROUP_CONTAINER_PADDING = 12.

const EDGE_GROUP_LABEL_PADDING = 20.

// min vertical distance between messages
const MIN_MESSAGE_DISTANCE = 30.

// default size
const SPAN_BASE_WIDTH = 12.

// as the spans start getting nested, their size grows
const SPAN_DEPTH_GROWTH_FACTOR = 8.

// when a span has a single messages
const MIN_SPAN_HEIGHT = 30.

const SPAN_MESSAGE_PAD = 10.

const LIFELINE_STROKE_WIDTH int = 2

const LIFELINE_STROKE_DASH int = 6

// pad when the actor has the label placed OutsideMiddleBottom so that the lifeline is not so close to the text
const LIFELINE_LABEL_PAD = 5.

const (
	LIFELINE_Z_INDEX = 1
	SPAN_Z_INDEX     = 2
	GROUP_Z_INDEX    = 3
	MESSAGE_Z_INDEX  = 4
	NOTE_Z_INDEX     = 5
)

// Helper functions to get appropriate constants based on ASCII mode
func getMinActorWidth(isASCII bool) float64 {
	if isASCII {
		return ASCII_MIN_ACTOR_WIDTH
	}
	return MIN_ACTOR_WIDTH
}

func getHorizontalPad(isASCII bool) float64 {
	if isASCII {
		return ASCII_HORIZONTAL_PAD
	}
	return HORIZONTAL_PAD
}

func getLabelHorizontalPad(isASCII bool) float64 {
	if isASCII {
		return ASCII_LABEL_HORIZONTAL_PAD
	}
	return LABEL_HORIZONTAL_PAD
}

func getVerticalPad(isASCII bool) float64 {
	if isASCII {
		return ASCII_VERTICAL_PAD
	}
	return VERTICAL_PAD
}

func getMinActorDistance(isASCII bool) float64 {
	if isASCII {
		return ASCII_MIN_ACTOR_DISTANCE
	}
	return MIN_ACTOR_DISTANCE
}
