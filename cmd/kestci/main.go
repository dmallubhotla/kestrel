package main

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	SetBuildInfo(version, commit, date)
	Execute()
}
