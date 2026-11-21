package main

import "runtime"

// otherThan returns goos, unless that is the platform running the test, in
// which case it returns plan9. A fixture named for a fixed OS (a_windows.gon,
// //go:build windows) is left out of the build on Linux and macOS but is a
// member of it on Windows, so a test that needs "a file the go tool leaves
// out" asks for a name that is not this platform's. On the platforms where the
// fixtures were written the name is unchanged.
func otherThan(goos string) string {
	if goos == runtime.GOOS {
		return "plan9"
	}
	return goos
}
