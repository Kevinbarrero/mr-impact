package gen

// List is a generic container.
type List[T any] struct {
	items []T
}

func (l *List[T]) Push(v T) {
	l.items = append(l.items, v)
}

// Use calls Push from a closure, on two instantiations.
func Use() int {
	a, b := &List[int]{}, &List[string]{}
	func() {
		a.Push(1)
		b.Push("x")
	}()
	return len(a.items) + len(b.items)
}
