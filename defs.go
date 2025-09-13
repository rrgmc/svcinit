package svcinit

import "errors"

type multiError struct {
	Errors []error
}

func (e *multiError) Error() string {
	err := e.Err()
	if err == nil {
		return "empty errors"
	}
	return err.Error()
}

func (e *multiError) Err() error {
	return errors.Join(e.Errors...)
}

func (e *multiError) Unwrap() []error {
	return e.Errors
}
