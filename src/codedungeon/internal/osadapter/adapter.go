// Package osadapter abstracts OS-specific behavior (path conventions, shell,
// executable resolution) behind a single interface. Commands call the adapter
// instead of `exec.Command` or `runtime.GOOS` directly — dependency inversion.
//
// Three concrete impls selected at build time via build tags:
//   - linux.go   //go:build linux
//   - windows.go //go:build windows
//   - darwin.go  //go:build darwin
//
// Factory `Detect()` returns the appropriate instance based on runtime.GOOS
// (kept for clarity even though build tags already enforce the choice).
package osadapter

// Adapter is the narrow OS-abstraction used by `codedungeon` subcommands.
type Adapter interface {
	// OS returns the canonical OS name: "linux", "windows", or "darwin".
	OS() string

	// PathSep returns the OS path list separator (":" on unix, ";" on windows).
	PathSep() string

	// HomeDir returns the user's home directory.
	HomeDir() string

	// TempDir returns the OS temp directory.
	TempDir() string

	// ExecutableExt returns the expected executable suffix: "" on unix, ".exe" on windows.
	ExecutableExt() string

	// FindTool resolves a tool by name on the PATH, with OS-specific hints
	// (e.g. gh.exe lives under /c/tools/gh on loldi's WSL2 setup).
	FindTool(name string) (string, error)

	// RunShell executes a shell command in `dir` and captures stdout/stderr.
	// Uses `bash -c` on unix, `cmd /c` on windows.
	RunShell(dir string, cmd string) (stdout, stderr string, err error)

	// RunExec runs a specific executable with args (no shell interpolation).
	RunExec(dir string, name string, args ...string) (stdout, stderr string, err error)
}

// Current returns the adapter for the runtime OS.
// Build tags guarantee only one of linux.go/windows.go/darwin.go is compiled in;
// each registers itself via Detect().
var _ Adapter = (Adapter)(nil) // compile-time: interface satisfied
