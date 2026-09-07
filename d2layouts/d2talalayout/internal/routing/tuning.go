package routing

const (
	basicallyInfinity              = 10_000_000.0
	extraInterestingPointLayers    = 3.0
	ovgPadding                     = 20.0
	overshootAmount                = 100.0
	pathNodeProximityFloor         = 5.0
	nodeProximityThreshold         = 30.0
	idealTurnAxisTolerance         = 4.0
	idealTurnMultiplier            = 0.98
	idealTurnEvenClusterMultiplier = 0.10
	nodeProximityPenalty           = 1.0
	turnEndpointClearance          = 20.0
	lowerJitterThreshold           = 5.0
	minStraightEdgeAngle           = 30.0
	nonOrthogonalFactor            = 4.0
	turnPenalty                    = 1.4
	segmentSpacingBuffer           = 40.0
	unboundedSegmentMove           = 100.0
	treeChildAlignmentTolerance    = 10.0
)
