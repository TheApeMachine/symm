package tests

import "github.com/theapemachine/symm/types"

/*
MockReporter is a controllable types.StatusReporter for booter and stage
tests. Status is driven directly by the test; Initialize records that it
ran and, when configured, drives the reporter to READY or ERROR itself so
sequencing tests can assert one reporter finished before the next starts.
*/
type MockReporter struct {
	status            types.Status
	initializeErr     error
	initializeCalls   int
	readyOnInitialize bool
}

/*
NewMockReporter constructs a MockReporter starting at status.
*/
func NewMockReporter(status types.Status) *MockReporter {
	return &MockReporter{status: status}
}

func (reporter *MockReporter) Status() types.Status {
	return reporter.status
}

/*
SetStatus lets a test move the reporter directly to a new status.
*/
func (reporter *MockReporter) SetStatus(status types.Status) {
	reporter.status = status
}

/*
Initialize satisfies types.StatusReporter. It records the call, applies
SetInitializeError if configured, and otherwise moves the reporter to
READY when SetReadyOnInitialize was requested.
*/
func (reporter *MockReporter) Initialize() error {
	reporter.initializeCalls++

	if reporter.initializeErr != nil {
		reporter.status = types.ERROR
		return reporter.initializeErr
	}

	if reporter.readyOnInitialize {
		reporter.status = types.READY
	}

	return nil
}

/*
SetInitializeError configures the error Initialize returns and moves the
reporter to ERROR when Initialize is next called.
*/
func (reporter *MockReporter) SetInitializeError(err error) {
	reporter.initializeErr = err
}

/*
SetReadyOnInitialize configures whether Initialize moves the reporter to
READY, mirroring a real reporter whose own Initialize does the work that
makes it ready.
*/
func (reporter *MockReporter) SetReadyOnInitialize(ready bool) {
	reporter.readyOnInitialize = ready
}

/*
InitializeCalls returns how many times Initialize was called.
*/
func (reporter *MockReporter) InitializeCalls() int {
	return reporter.initializeCalls
}
