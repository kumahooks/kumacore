// Package enqueuestub provides a test double for the dispatch enqueuer
// interface. It records the last job name and payload, and counts calls,
// so handler tests can assert dispatch behavior without a running worker.
package enqueuestub

// Stub records enqueue calls for assertion in tests.
type Stub struct {
	Err         error
	Calls       int
	LastJobName string
	LastPayload any
}

// Enqueue records the call and returns the configured error.
func (stub *Stub) Enqueue(jobName string, payload any) error {
	stub.Calls++
	stub.LastJobName = jobName
	stub.LastPayload = payload

	return stub.Err
}
