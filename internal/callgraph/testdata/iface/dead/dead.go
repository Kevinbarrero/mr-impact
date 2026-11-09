package dead

// Used is called by Entry.
func Used() int { return 1 }

// Entry is the package's exported entry point.
func Entry() int { return Used() + kept() }

func kept() int { return 2 }

// lost has no callers left.
func lost() int { return 3 + helper() }

// helper is called only by lost.
func helper() int { return 0 }

// testOnly is called only from tests.
func testOnly() int { return 4 }

type t struct{}

// Exported methods may satisfy interfaces of dependencies: never reported.
func (t) String() string { return "" }

func (t) gone() {}

// registered is referenced only as a value in a package-level map.
func registered() {}

var table = map[string]func(){"r": registered}

func init() { _ = table }
