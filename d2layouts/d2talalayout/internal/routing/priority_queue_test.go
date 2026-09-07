package routing

import (
	"errors"
	"fmt"
	"sort"
	"testing"
)

var priorityQueueBenchmarkSink float64

type rejectingPriorityQueueWorkBudget struct {
	err error
}

func (g rejectingPriorityQueueWorkBudget) step() error      { return g.err }
func (g rejectingPriorityQueueWorkBudget) add(uint64) error { return g.err }
func (g rejectingPriorityQueueWorkBudget) check() error     { return g.err }

func TestPriorityQueue(t *testing.T) {
	var queue priorityQueue

	large, err := queue.push(3, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	medium, err := queue.push(2, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	small, err := queue.push(1, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []*priorityQueueEntry{small, medium, large} {
		got, err := queue.pop(nil)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("pop returned %p; want %p", got, want)
		}
	}
	if !queue.empty() {
		t.Fatal("queue is not empty after popping every entry")
	}
}

func TestPriorityQueueDecrease(t *testing.T) {
	var queue priorityQueue

	entry, err := queue.push(3, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := queue.push(2, nil, false, nil); err != nil {
		t.Fatal(err)
	}
	if err := queue.decrease(entry, 1, nil); err != nil {
		t.Fatal(err)
	}

	got, err := queue.pop(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != entry {
		t.Fatalf("pop returned %p after decrease; want %p", got, entry)
	}
	if err := queue.decrease(entry, 0, nil); err == nil {
		t.Fatal("decreasing a popped entry succeeded")
	}
}

func TestPriorityQueueDecreasePreservesOrder(t *testing.T) {
	var queue priorityQueue

	first, err := queue.push(1, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := queue.push(2, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := queue.decrease(second, 1, nil); err != nil {
		t.Fatal(err)
	}

	for _, want := range []*priorityQueueEntry{first, second} {
		got, err := queue.pop(nil)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("pop returned %p; want %p", got, want)
		}
	}
}

func TestPriorityQueueGuardFailureDoesNotMutate(t *testing.T) {
	errRejected := errors.New("reject priority queue work")
	guard := rejectingPriorityQueueWorkBudget{err: errRejected}

	t.Run("push", func(t *testing.T) {
		var queue priorityQueue
		entry, err := queue.push(1, nil, false, guard)
		if !errors.Is(err, errRejected) {
			t.Fatalf("push error = %v; want %v", err, errRejected)
		}
		if entry != nil {
			t.Fatalf("push entry = %p; want nil", entry)
		}
		if len(queue.items) != 0 || queue.entryCursor != 0 || queue.nextOrder != 0 || len(queue.entryChunks) != 0 {
			t.Fatalf("push mutated queue: %+v", queue)
		}
	})

	t.Run("pop", func(t *testing.T) {
		var queue priorityQueue
		first, err := queue.push(1, nil, false, nil)
		if err != nil {
			t.Fatal(err)
		}
		second, err := queue.push(2, nil, false, nil)
		if err != nil {
			t.Fatal(err)
		}

		entry, err := queue.pop(guard)
		if !errors.Is(err, errRejected) {
			t.Fatalf("pop error = %v; want %v", err, errRejected)
		}
		if entry != nil {
			t.Fatalf("pop entry = %p; want nil", entry)
		}
		if len(queue.items) != 2 || queue.items[0] != first || queue.items[1] != second || first.index != 0 || second.index != 1 {
			t.Fatalf("pop mutated queue: %+v", queue.items)
		}
	})

	t.Run("decrease", func(t *testing.T) {
		var queue priorityQueue
		entry, err := queue.push(2, nil, false, nil)
		if err != nil {
			t.Fatal(err)
		}

		err = queue.decrease(entry, 1, guard)
		if !errors.Is(err, errRejected) {
			t.Fatalf("decrease error = %v; want %v", err, errRejected)
		}
		if entry.priority != 2 || entry.index != 0 || len(queue.items) != 1 || queue.items[0] != entry {
			t.Fatalf("decrease mutated queue: entry=%+v items=%+v", entry, queue.items)
		}
	})
}

func TestPriorityQueueEqualPriorityOrder(t *testing.T) {
	var queue priorityQueue

	horizontalFirst, err := queue.push(1, nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	verticalFirst, err := queue.push(1, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	horizontalSecond, err := queue.push(1, nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	verticalSecond, err := queue.push(1, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}

	want := []*priorityQueueEntry{horizontalFirst, verticalFirst, horizontalSecond, verticalSecond}
	for _, wantEntry := range want {
		got, err := queue.pop(nil)
		if err != nil {
			t.Fatal(err)
		}
		if got != wantEntry {
			t.Fatalf("pop returned %p; want %p", got, wantEntry)
		}
	}
}

func TestPriorityQueueResetReusesEntries(t *testing.T) {
	var queue priorityQueue

	first, err := queue.push(1, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	queue.reset()
	second, err := queue.push(2, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if second != first {
		t.Fatalf("reset allocated entry %p; want reused entry %p", second, first)
	}
}

func TestPriorityQueueAcrossEntryChunks(t *testing.T) {
	const size = priorityQueueEntryChunkSize*2 + 1

	var queue priorityQueue
	type modelEntry struct {
		entry    *priorityQueueEntry
		priority float64
		order    int
	}
	model := make([]modelEntry, size)
	for i := range model {
		priority := float64((i * 37) % 100)
		entry, err := queue.push(priority, nil, i%2 == 0, nil)
		if err != nil {
			t.Fatal(err)
		}
		model[i] = modelEntry{entry: entry, priority: priority, order: i}
	}
	for i := 0; i < len(model); i += 17 {
		priority := -float64(i + 1)
		if err := queue.decrease(model[i].entry, priority, nil); err != nil {
			t.Fatal(err)
		}
		model[i].priority = priority
	}

	sort.Slice(model, func(i, j int) bool {
		if model[i].priority != model[j].priority {
			return model[i].priority < model[j].priority
		}
		return model[i].order < model[j].order
	})
	for _, want := range model {
		got, err := queue.pop(nil)
		if err != nil {
			t.Fatal(err)
		}
		if got != want.entry {
			t.Fatalf("pop returned %p; want %p", got, want.entry)
		}
	}
}

func BenchmarkPriorityQueueMixed(b *testing.B) {
	for _, size := range []int{64, 256, 1024, 4096} {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			entries := make([]*priorityQueueEntry, size)
			var queue priorityQueue
			b.ReportAllocs()

			var sum float64
			for b.Loop() {
				queue.reset()
				for i := range entries {
					var err error
					entries[i], err = queue.push(float64(size+i), nil, false, nil)
					if err != nil {
						b.Fatal(err)
					}
				}
				for i := 0; i < len(entries); i += 3 {
					if err := queue.decrease(entries[i], float64(i)/2, nil); err != nil {
						b.Fatal(err)
					}
				}
				for !queue.empty() {
					entry, err := queue.pop(nil)
					if err != nil {
						b.Fatal(err)
					}
					sum += entry.priority
				}
			}
			priorityQueueBenchmarkSink = sum
		})
	}
}
