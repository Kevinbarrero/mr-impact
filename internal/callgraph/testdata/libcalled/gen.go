package lc

type E[T any] struct{ v T }

func (E[T]) Unwrap() error { return errBase }

func (E[T]) Error() string { return "e" }
