package placement

import "github.com/d2lang/d2/d2layouts/d2talalayout/internal/typedpool"

type edgeAbductionBoolScratch struct {
	values []bool
}

var edgeAbductionBoolScratchPool = typedpool.New(func() *edgeAbductionBoolScratch {
	return &edgeAbductionBoolScratch{values: make([]bool, 0, 16)}
})

func borrowEdgeAbductionBools(length int) *edgeAbductionBoolScratch {
	scratch := edgeAbductionBoolScratchPool.Get()
	if cap(scratch.values) < length {
		scratch.values = make([]bool, length)
		return scratch
	}
	scratch.values = scratch.values[:length]
	clear(scratch.values)
	return scratch
}

func returnEdgeAbductionBools(scratch *edgeAbductionBoolScratch) {
	scratch.values = scratch.values[:0]
	edgeAbductionBoolScratchPool.Put(scratch)
}
