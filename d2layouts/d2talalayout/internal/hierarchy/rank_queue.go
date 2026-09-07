package hierarchy

// nodeMinHeap is a small, allocation-free priority queue for stable Kahn
// traversal. Node indexes already follow entity-ID order.
type nodeMinHeap []int

func (h *nodeMinHeap) push(node int) {
	*h = append(*h, node)
	for child := len(*h) - 1; child > 0; {
		parent := (child - 1) / 2
		if (*h)[parent] <= (*h)[child] {
			break
		}
		(*h)[parent], (*h)[child] = (*h)[child], (*h)[parent]
		child = parent
	}
}

func (h *nodeMinHeap) pop() int {
	old := *h
	root := old[0]
	last := old[len(old)-1]
	old = old[:len(old)-1]
	if len(old) > 0 {
		old[0] = last
		for parent := 0; ; {
			left := parent*2 + 1
			if left >= len(old) {
				break
			}
			child := left
			right := left + 1
			if right < len(old) && old[right] < old[left] {
				child = right
			}
			if old[parent] <= old[child] {
				break
			}
			old[parent], old[child] = old[child], old[parent]
			parent = child
		}
	}
	*h = old
	return root
}
