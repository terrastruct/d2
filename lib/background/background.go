package background

import "time"

func Repeat(do func(), interval time.Duration) (cancel func()) {
	t := time.NewTicker(interval)
	done := make(chan struct{})

	go func() {
		defer t.Stop()
		for {
			select {
			case <-t.C:
				do()
			case <-done:
				return
			}
		}
	}()

	stopped := false
	return func() {
		if !stopped {
			stopped = true
			close(done)
		}
	}
}
