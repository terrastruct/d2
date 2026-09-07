package routing

import (
	"fmt"
	"math/bits"
)

const priorityQueueEntryChunkSize = 256

// priorityQueue is an indexed binary min-heap. Entries keep their heap index
// so Dijkstra's algorithm can lower a queued distance without a map lookup.
// Equal priorities are popped in insertion order.
type priorityQueue struct {
	items       []*priorityQueueEntry
	entryChunks [][]priorityQueueEntry
	entryCursor int
	nextOrder   uint64
}

type priorityQueueEntry struct {
	node         *OVGNode
	isHorizontal bool
	priority     float64
	index        int
	order        uint64
}

// reset empties the queue while retaining storage. Entries returned before a
// reset must not be used afterward.
func (q *priorityQueue) reset() {
	clear(q.items)
	q.items = q.items[:0]
	q.entryCursor = 0
	q.nextOrder = 0
}

func (q *priorityQueue) empty() bool {
	return len(q.items) == 0
}

func (q *priorityQueue) push(priority float64, node *OVGNode, isHorizontal bool, guard workBudget) (*priorityQueueEntry, error) {
	if err := reservePriorityQueueWork(guard, len(q.items)+1, 1); err != nil {
		return nil, err
	}

	entry := q.allocateEntry(priority)
	entry.node = node
	entry.isHorizontal = isHorizontal
	entry.index = len(q.items)
	entry.order = q.nextOrder
	q.nextOrder++
	q.items = append(q.items, entry)
	q.siftUp(entry.index)
	return entry, nil
}

func (q *priorityQueue) pop(guard workBudget) (*priorityQueueEntry, error) {
	if q.empty() {
		return nil, fmt.Errorf("cannot dequeue minimum of empty priority queue")
	}
	if err := reservePriorityQueueWork(guard, len(q.items), 2); err != nil {
		return nil, err
	}

	min := q.items[0]
	lastIndex := len(q.items) - 1
	last := q.items[lastIndex]
	q.items[lastIndex] = nil
	q.items = q.items[:lastIndex]
	min.index = -1

	if lastIndex != 0 {
		q.items[0] = last
		last.index = 0
		q.siftDown(0)
	}
	return min, nil
}

func (q *priorityQueue) decrease(entry *priorityQueueEntry, priority float64, guard workBudget) error {
	if q.empty() {
		return fmt.Errorf("cannot decrease priority in an empty priority queue")
	}
	if entry == nil {
		return fmt.Errorf("cannot decrease priority: entry is nil")
	}
	if entry.index < 0 || entry.index >= len(q.items) || q.items[entry.index] != entry {
		return fmt.Errorf("cannot decrease priority: entry is not in the priority queue")
	}
	if priority >= entry.priority {
		return fmt.Errorf("new priority %v is not less than old priority %v", priority, entry.priority)
	}
	if err := reservePriorityQueueWork(guard, entry.index+1, 1); err != nil {
		return err
	}

	entry.priority = priority
	q.siftUp(entry.index)
	return nil
}

func reservePriorityQueueWork(guard workBudget, upperBound, unitsPerLevel int) error {
	if guard == nil {
		return nil
	}
	return guard.add(uint64(bits.Len(uint(upperBound)) * unitsPerLevel))
}

func (q *priorityQueue) allocateEntry(priority float64) *priorityQueueEntry {
	chunkIndex := q.entryCursor / priorityQueueEntryChunkSize
	offset := q.entryCursor % priorityQueueEntryChunkSize
	if chunkIndex == len(q.entryChunks) {
		q.entryChunks = append(q.entryChunks, make([]priorityQueueEntry, priorityQueueEntryChunkSize))
	}
	q.entryCursor++

	entry := &q.entryChunks[chunkIndex][offset]
	*entry = priorityQueueEntry{priority: priority, index: -1}
	return entry
}

func (q *priorityQueue) siftUp(index int) {
	for index > 0 {
		parent := (index - 1) / 2
		if !q.less(index, parent) {
			return
		}
		q.swap(index, parent)
		index = parent
	}
}

func (q *priorityQueue) siftDown(index int) {
	for {
		left := index*2 + 1
		if left >= len(q.items) {
			return
		}
		smallest := left
		right := left + 1
		if right < len(q.items) && q.less(right, left) {
			smallest = right
		}
		if !q.less(smallest, index) {
			return
		}
		q.swap(index, smallest)
		index = smallest
	}
}

func (q *priorityQueue) less(left, right int) bool {
	return priorityQueueEntryLess(q.items[left], q.items[right])
}

func priorityQueueEntryLess(a, b *priorityQueueEntry) bool {
	if a.priority != b.priority {
		return a.priority < b.priority
	}
	// Preserve insertion order for equal-cost states. Route selection should not
	// depend on the binary heap's internal shape.
	return a.order < b.order
}

func (q *priorityQueue) swap(left, right int) {
	q.items[left], q.items[right] = q.items[right], q.items[left]
	q.items[left].index = left
	q.items[right].index = right
}
