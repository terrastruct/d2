package d2talalayout

import "github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"

// Keep adapter preflight aligned with the native engine limits.
const (
	maxInputNodes       = limits.MaxEngineNodes
	maxInputEdges       = limits.MaxEngineEdges
	maxInputRoutePoints = limits.MaxEngineRoutePoints
	maxInputTreeDepth   = limits.MaxEngineTreeDepth

	// maxResultCoordinate keeps geometry within the range safely consumed by
	// D2's integer render dimensions while remaining far above practical space.
	maxResultCoordinate = 1_000_000_000
)
