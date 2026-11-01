//go:build !unix

package main

// execProgram is never reached on a system without exec: run only builds a
// bundle on darwin and runs everything else through runPlain.
func execProgram(exe string, args []string) error {
	return runPlain(exe, args)
}
