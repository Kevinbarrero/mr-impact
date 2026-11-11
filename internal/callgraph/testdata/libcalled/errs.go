package lc

import "errors"

var errBase = errors.New("base")

type candidateErrors struct{}

func (candidateErrors) Unwrap() error { return errBase }

func (candidateErrors) Error() string { return "candidates" }
