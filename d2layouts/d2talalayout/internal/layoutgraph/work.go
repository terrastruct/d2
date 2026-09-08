package layoutgraph

import "github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"

const (
	// Keep the established engine-local names while the surrounding layout
	// domains migrate independently. The values and their implementation now
	// have one cycle-free owner.
	maxEngineNodes                  = limits.MaxEngineNodes
	maxEngineEdges                  = limits.MaxEngineEdges
	maxEngineWorkUnits              = limits.MaxEngineWorkUnits
	maxTransactionWorkUnits         = limits.MaxTransactionWorkUnits
	maxTransactionOverlapReferences = limits.MaxTransactionOverlapReferences
	maxBinPackWorkUnits             = limits.MaxBinPackWorkUnits
	maxGraphSize                    = limits.MaxGraphSize
	maxCompactionCandidateCount     = limits.MaxGraphSize + 3
)

// workStepper is the narrow accounting contract used by graph traversals.
// Concrete operation budgets use limits.WorkGuard directly; tests and sibling
// domains may provide another implementation without an adapter.
type workStepper interface {
	Step() error
	Finish() error
}

type unboundedWorkStepper struct{}

func (*unboundedWorkStepper) Step() error   { return nil }
func (*unboundedWorkStepper) Finish() error { return nil }

var unboundedWork = &unboundedWorkStepper{}
