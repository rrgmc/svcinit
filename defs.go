package svcinit

import "errors"

type MultiError struct {
	Errors []error
}

func (e *MultiError) Error() string {
	err := e.Err()
	if err == nil {
		return "empty errors"
	}
	return err.Error()
}

func (e *MultiError) Err() error {
	return errors.Join(e.Errors...)
}

func (e *MultiError) Unwrap() []error {
	return e.Errors
}
