package main

import "github.com/example/kestrel/cmd"

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	cmd.SetBuildInfo(version, commit, date)
	cmd.Execute()
}
