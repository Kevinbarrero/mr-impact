package describe

import "os/exec"

// Tool finds the named tool on PATH, or returns the name unchanged.
func Tool(name string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return name
}
