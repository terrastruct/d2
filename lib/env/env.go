package env

import (
	"os"
	"strconv"
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
	return os.Getenv("SKIP_GRAPH_DIFF_TESTS") != ""
}

func Timeout() (int, bool) {
	if s := os.Getenv("D2_TIMEOUT"); s != "" {
		i, err := strconv.ParseInt(s, 10, 64)
		if err == nil {
			return int(i), true
		}
	}
	return -1, false
}
