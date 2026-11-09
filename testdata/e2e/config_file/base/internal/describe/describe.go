package describe

import "os/exec"

// Tool finds the named tool on PATH.
func Tool(name string) string {
	p, _ := exec.LookPath(name)
	return p
}
