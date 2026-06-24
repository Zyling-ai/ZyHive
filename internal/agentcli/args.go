package agentcli

import (
	"flag"
	"io"
)

// newFlagSet returns a FlagSet that reports errors to us (not stderr) so the
// CLI can render them consistently with the rest of the output.
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// parseFlags parses args while tolerating flags and positionals in any order
// (the stdlib flag package stops at the first positional). It repeatedly parses,
// peeling one positional at a time, so `cmd a --flag b` works like `cmd --flag a b`.
func parseFlags(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	cur := args
	for {
		if err := fs.Parse(cur); err != nil {
			return nil, usageErr("%v", err)
		}
		rest := fs.Args()
		if len(rest) == 0 {
			break
		}
		positional = append(positional, rest[0])
		cur = rest[1:]
	}
	return positional, nil
}

// arg returns the i-th positional or "" if out of range.
func arg(positional []string, i int) string {
	if i < 0 || i >= len(positional) {
		return ""
	}
	return positional[i]
}
