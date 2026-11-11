package sc

type candidateErrors []error

func (c candidateErrors) Error() string { return "candidates failed" }
