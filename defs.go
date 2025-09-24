package svcinit

import (
	"errors"
	"fmt"
)

var (
	ErrExit               = errors.New("normal exit")
	ErrInvalidStage       = errors.New("invalid stage")
	ErrInvalidTaskStep    = errors.New("invalid step for task")
	ErrInvalidStepOrder   = errors.New("invalid step order")
	ErrAlreadyRunning     = errors.New("already running")
	ErrInitialization     = errors.New("initialization error")
	ErrNoStartTask        = errors.New("no start tasks available")
	ErrNilTask            = errors.New("nil task")
	ErrNoStage            = errors.New("no stages available")
	ErrShutdownTimeout    = errors.New("shutdown timeout")
	ErrAlreadyInitialized = errors.New("already initialized")
	ErrNotInitialized     = errors.New("not initialized")
	ErrDuplicateStep      = errors.New("duplicate step")
)

const (
	StageDefault = "default"
)

type Step int

const (
	StepSetup Step = iota
	StepStart
	StepPreStop
	StepStop
	StepTeardown
)

func (s Step) String() string {
	switch s {
	case StepSetup:
		return "setup"
	case StepStart:
		return "start"
	case StepStop:
		return "stop"
	case StepPreStop:
		return "pre-stop"
	case StepTeardown:
		return "teardown"
	default:
		return "unknown-step"
	}
}

type CallbackStep int

const (
	CallbackStepBefore CallbackStep = iota
	CallbackStepAfter
)

func (s CallbackStep) String() string {
	switch s {
	case CallbackStepBefore:
		return "before"
	case CallbackStepAfter:
		return "after"
	default:
		return "unknown-callback-step"
	}
}

func newInvalidStage(stage string) error {
	if stage == "" {
		return fmt.Errorf("%w: '' (blank)", ErrInvalidStage)
	}
	return fmt.Errorf("%w: '%s'", ErrInvalidStage, stage)
}

func newInvalidTaskStep(step Step) error {
	return fmt.Errorf("%w: %s", ErrInvalidTaskStep, step)
}

func newInitializationError(err error) error {
	return fmt.Errorf("%w: %w", ErrInitialization, err)
}

// fatalError is an internal error signaling that a fatal error happened and must be reported in the Run cause.
type fatalError struct {
	err error
}

func (f fatalError) Error() string {
	return f.err.Error()
}

func (f fatalError) Unwrap() error {
	return f.err
}

// unwrapInternalErrors unwraps fatalError from the error, if any.
func unwrapInternalErrors(err error) error {
	for {
		if err == nil {
			break
		}
		var ferr fatalError
		if errors.As(err, &ferr) {
			err = ferr.err
			continue
		}
		break
	}
	return err
}
