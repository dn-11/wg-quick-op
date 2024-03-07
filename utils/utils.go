package utils

import "time"

func Retry(times int, f func() error) <-chan error {
	done := make(chan error)
	go func() {
		wait := time.Second
		var err error
		for times > 0 {
			err = f()
			if err == nil {
				break
			}
			time.Sleep(wait)
			wait = wait * 2
			times--
		}
		if err != nil {
			done <- err
		}
		close(done)
	}()
	return done
}
