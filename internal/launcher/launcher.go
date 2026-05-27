// Package launcher constructs and executes the `claude --resume <uuid>`
// invocation. Exec replaces the current process so the TUI vanishes and the
// resumed chat takes over the same terminal window.
package launcher

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// Options controls how Resume is invoked.
type Options struct {
	UUID        string
	Cwd         string
	DefaultArgs []string
	ForkSession bool
}

// BuildArgs returns the full argv (including argv[0]="claude") for the
// resume invocation. ForkSession appends --fork-session; DefaultArgs are
// appended last so users can override behavior via config.
func BuildArgs(opts Options) []string {
	args := []string{"claude", "--resume", opts.UUID}
	if opts.ForkSession {
		args = append(args, "--fork-session")
	}
	args = append(args, opts.DefaultArgs...)
	return args
}

// Exec chdir's into Cwd (if accessible) and replaces the current process
// with `claude --resume <uuid> ...`. On success it does not return.
func Exec(opts Options) error {
	if opts.Cwd != "" {
		if _, err := os.Stat(opts.Cwd); err != nil {
			return fmt.Errorf("cwd %q not accessible: %w", opts.Cwd, err)
		}
		if err := os.Chdir(opts.Cwd); err != nil {
			return err
		}
	}
	bin, err := exec.LookPath("claude")
	if err != nil {
		return fmt.Errorf("claude not found in PATH: %w", err)
	}
	return syscall.Exec(bin, BuildArgs(opts), os.Environ())
}
