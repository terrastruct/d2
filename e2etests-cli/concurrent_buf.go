package e2etests_cli

import "sync"

// stderrWrapper lets stderr be read/write concurrently
type stderrWrapper struct {
	msg string
	m   sync.Mutex
}

func (e *stderrWrapper) Write(p []byte) (n int, err error) {
	e.m.Lock()
	defer e.m.Unlock()
	e.msg += string(p)
	return len(p), nil
}

func (e *stderrWrapper) Reset() {
	e.msg = ""
}

func (e *stderrWrapper) Read() string {
	e.m.Lock()
	defer e.m.Unlock()
	return e.msg
}
