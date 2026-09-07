package packing

type pointerSnapshot[T any] struct {
	pointer *T
	value   T
}

func snapshotPointer[T any](pointer *T) pointerSnapshot[T] {
	if pointer == nil {
		return pointerSnapshot[T]{}
	}
	return pointerSnapshot[T]{pointer: pointer, value: *pointer}
}

func (snapshot pointerSnapshot[T]) restore() *T {
	if snapshot.pointer == nil {
		return nil
	}
	*snapshot.pointer = snapshot.value
	return snapshot.pointer
}
