package llm

import "sync"

// Job is work running in the background, such as a request to the model.
// The game loop checks Done each tick and never waits for it.
type Job[T any] struct {
	mu   sync.Mutex
	done bool
	val  T
	err  error
}

// Start runs fn in the background.
func Start[T any](fn func() (T, error)) *Job[T] {
	j := &Job[T]{}
	go func() {
		v, err := fn()
		j.mu.Lock()
		j.val, j.err, j.done = v, err, true
		j.mu.Unlock()
	}()
	return j
}

// Done reports whether the job has finished. A nil job is never done.
func (j *Job[T]) Done() bool {
	if j == nil {
		return false
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.done
}

// Result returns what the job made. It is only meaningful once Done.
func (j *Job[T]) Result() (T, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.val, j.err
}
