//go:build !linux && !darwin

package sandbox

import (
	"context"
	"os/exec"
)

// On non-Unix platforms (windows, plan9, ...) we cannot Setpgid or apply
// POSIX rlimits in a meaningful way. The sandbox degrades to "exec via
// context.WithTimeout only" — strictly better than the legacy no-sandbox
// path because of the tmp HOME isolation, but with no kernel-enforced
// resource ceiling.

func applySysProcAttr(_ *exec.Cmd, _ Limits) {
	// nothing to do
}

func configureProcessGroupKill(_ context.Context, _ *exec.Cmd) func() {
	return func() {}
}

func classifyKillReason(_ *exec.ExitError) string { return "" }
