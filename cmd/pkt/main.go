package main

import (
	"github.com/patchkit-net/patchkit-tools-go/internal/cli"
)

// Set via ldflags at build time.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cli.Version = version
	cli.Commit = commit
	cli.Date = date
	cli.Execute()
}
