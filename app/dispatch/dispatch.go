// Package dispatch provides a work dispatcher for triggering background jobs
// from within module handlers without direct coupling to the worker package.
package dispatch

type enqueuer interface {
	Enqueue(jobName string, payload any) error
}

type noopEnqueuer struct{}

func (noopEnqueuer) Enqueue(_ string, _ any) error {
	return nil
}

type Dispatcher struct {
	runner enqueuer
}

func NewNoop() *Dispatcher {
	return New(noopEnqueuer{})
}

func New(runner enqueuer) *Dispatcher {
	return &Dispatcher{runner: runner}
}

func (dispatcher *Dispatcher) Dispatch(jobName string, payload any) error {
	return dispatcher.runner.Enqueue(jobName, payload)
}
