package run

import "os/exec"

// Name is the runner's name.
func Name() string { return "run" }

// Exec runs a command: new, no callers, no test.
func Exec(name string) error {
	if name == "" {
		return nil
	}
	return exec.Command(name).Run()
}
