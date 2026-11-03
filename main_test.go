package main

import (
	"slices"
	"testing"
)

func TestCommand(t *testing.T) {
	for _, c := range []struct {
		in   []string
		cmd  string
		args []string
	}{
		{nil, "app", nil},
		{[]string{"mcp", "-data", "/tmp/x"}, "mcp", []string{"-data", "/tmp/x"}},
		{[]string{"-data", "/tmp/x"}, "app", []string{"-data", "/tmp/x"}},
		{[]string{"-psn_0_12345"}, "app", nil},
		// Windows's login start
		{[]string{"-login"}, "app", []string{"-login"}},
		// Windows starts the exe with the link as its only argument
		{[]string{"healthtrail://appointments/2?uid=0123456789abcdef0123456789abcdef"}, "app", nil},
		// a link can't smuggle in a data directory
		{[]string{"healthtrail://x", "-data", `\\host\share`}, "app", nil},
	} {
		cmd, args := command(c.in)
		if cmd != c.cmd || !slices.Equal(args, c.args) {
			t.Errorf("command(%q) = %q %q, want %q %q", c.in, cmd, args, c.cmd, c.args)
		}
	}
}
