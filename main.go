// agentop — htop for AI coding agents.
//
// Discovers running Claude Code (and, later, other agent) sessions across
// cmux, emdash, and plain terminals; shows memory + idle age + owning task;
// and lets you selectively reap the idle ones to reclaim RAM.
//
// Build:   go build -o bin/agentop .
// Run:     ./bin/agentop            # TUI
//
//	./bin/agentop list       # one-shot table
//	./bin/agentop reap --idle 2
package main

import (
	"runtime/debug"

	"github.com/thesatellite-ai/agentop/internal/cli"
)

// version is injected by goreleaser at build time via
// -ldflags '-X main.version=<value>'. It is empty for `go install` and plain
// `go build`, where resolveVersion() falls back to the module's build info.
var version = ""

func main() {
	cli.SetVersion(resolveVersion())
	cli.Execute()
}

// resolveVersion reports the build version, preferring the ldflags-injected
// value (release builds), then the module version embedded by `go install`
// (e.g. "v0.1.2"), and finally "dev" for an un-tagged local build.
func resolveVersion() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		if v := bi.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return "dev"
}
