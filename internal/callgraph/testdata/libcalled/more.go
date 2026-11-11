package lc

// S is printed by a test through fmt.Println(v).
type S struct{}

func (S) String() string { return "s" }

// Q has a pointer-receiver String; a test converts &Q{}.
type Q struct{}

func (*Q) String() string { return "q" }

// C is converted to error inside a closure of a test.
type C struct{}

func (C) Error() string { return "c" }
