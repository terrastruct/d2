package env

import (
	"os"
)

func Test() bool {
	return os.Getenv("TEST_MODE") != ""
}

func Dev() bool {
	return os.Getenv("DEV_MODE") != ""
}

func Debug() bool {
	return os.Getenv("DEBUG") != ""
}

// People have DEV_MODE on while running tests. If that's the case, this
// function will return false.
func DevOnly() bool {
	return Dev() && !Test()
}

func SkipGraphDiffTests() bool {
	return os.Getenv("SKIP_GRAPH_DIFF_TESTS") == "on"
}
