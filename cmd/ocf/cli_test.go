package main

import (
	"bytes"
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runCommand executes the root command with args, capturing stdout and stderr.
func runCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()

	root := newRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)

	err := root.Execute()

	return out.String(), err
}

func TestRootCommandListsSubcommands(t *testing.T) {
	t.Parallel()

	out, err := runCommand(t, "--help")
	require.NoError(t, err)
	assert.Contains(t, out, "scaffold")
	assert.Contains(t, out, "version")
}

func TestVersionCommandPrintsVersion(t *testing.T) {
	t.Parallel()

	out, err := runCommand(t, "version")
	require.NoError(t, err)
	assert.NotEmpty(t, out)
}

func TestVersionFrom(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		info     *debug.BuildInfo
		ok       bool
		expected string
	}{
		{
			name:     "build info unavailable",
			info:     nil,
			ok:       false,
			expected: "unknown",
		},
		{
			name:     "empty main version",
			info:     &debug.BuildInfo{Main: debug.Module{Version: ""}},
			ok:       true,
			expected: "unknown",
		},
		{
			name:     "tagged version",
			info:     &debug.BuildInfo{Main: debug.Module{Version: "v1.2.3"}},
			ok:       true,
			expected: "v1.2.3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, versionFrom(tt.info, tt.ok))
		})
	}
}
