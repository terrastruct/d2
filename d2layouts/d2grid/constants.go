package d2grid

const (
	// don't consider layouts with rows longer than targetSize*1.2 or shorter than targetSize/1.2
	STARTING_THRESHOLD = 1.2
	// next try layouts with a 25% larger threshold
	THRESHOLD_STEP_SIZE    = 0.25
	MIN_THRESHOLD_ATTEMPTS = 1
	MAX_THRESHOLD_ATTEMPTS = 3

	ATTEMPT_LIMIT = 100_000
	SKIP_LIMIT    = 10_000_000
)
