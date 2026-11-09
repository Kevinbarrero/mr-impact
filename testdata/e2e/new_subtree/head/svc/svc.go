package svc

type S struct{ n int }

func New(n int) string {
	s := &S{n: load(n)}
	s.apply(price(s.n))
	return fmt(s.n)
}

func load(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

func price(n int) int {
	if n > 100 {
		return 100
	}
	return n * 2
}

func (s *S) apply(p int) {
	if p > 0 {
		s.n += p
	}
}

func fmt(n int) string {
	if n == 0 {
		return "zero"
	}
	return "some"
}
