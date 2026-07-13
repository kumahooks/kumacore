package dispatch_test

import (
	"errors"
	"testing"

	"kumacore/app/dispatch"
)

type mockEnqueuer struct {
	lastJobName string
	lastPayload any
	err         error
}

func (mock *mockEnqueuer) Enqueue(jobName string, payload any) error {
	mock.lastJobName = jobName
	mock.lastPayload = payload

	return mock.err
}

func TestDispatch_ForwardsJobNameAndPayload(t *testing.T) {
	enqueuer := &mockEnqueuer{}
	dispatcher := dispatch.New(enqueuer)

	if err := dispatcher.Dispatch("test:job", "mydata"); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	if enqueuer.lastJobName != "test:job" {
		t.Errorf("job name: got %q, want %q", enqueuer.lastJobName, "test:job")
	}

	if enqueuer.lastPayload != "mydata" {
		t.Errorf("payload: got %v, want %q", enqueuer.lastPayload, "mydata")
	}
}

func TestDispatch_PropagatesError(t *testing.T) {
	enqueuer := &mockEnqueuer{err: errors.New("unknown job")}
	dispatcher := dispatch.New(enqueuer)

	if err := dispatcher.Dispatch("test:unknown", nil); err == nil {
		t.Error("expected error from enqueuer, got nil")
	}
}

func TestNewNoop_DiscardsSilently(t *testing.T) {
	dispatcher := dispatch.NewNoop()

	if err := dispatcher.Dispatch("any:job", "any payload"); err != nil {
		t.Errorf("NewNoop Dispatch returned unexpected error: %v", err)
	}
}
