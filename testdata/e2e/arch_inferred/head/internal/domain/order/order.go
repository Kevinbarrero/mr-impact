package order

import "example.com/ai/internal/repository/pg"

// Total doubles the stored n.
func Total(n int) int { return pg.Load(n) * 2 }
