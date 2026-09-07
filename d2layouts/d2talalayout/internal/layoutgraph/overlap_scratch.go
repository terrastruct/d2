package layoutgraph

import "github.com/d2lang/d2/d2layouts/d2talalayout/internal/typedpool"

const maxPooledOverlapExceptions = 4096

type overlapExceptionScratch struct {
	nodes map[*Node]struct{}
}

var overlapExceptionPool = typedpool.New(func() *overlapExceptionScratch {
	return new(overlapExceptionScratch)
})

// Container transactions repeatedly validate the same descendant exceptions.
// Retain bounded map storage across checks, while releasing every graph pointer
// on success, cancellation, resource failure, or panic.
func releaseOverlapExceptions(scratch *overlapExceptionScratch) {
	if len(scratch.nodes) > maxPooledOverlapExceptions {
		scratch.nodes = nil
	} else {
		clear(scratch.nodes)
	}
	overlapExceptionPool.Put(scratch)
}
