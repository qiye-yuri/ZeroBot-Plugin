package goba

import "errors"

var ErrModelRequestFailed = errors.New("Agent model request failed")

// Keep causes available for errors.Is/As (notably host response limits), while
// the default error text never includes provider bodies, URLs or credentials.
type modelRequestError struct{ cause error }

func (*modelRequestError) Error() string        { return ErrModelRequestFailed.Error() }
func (e *modelRequestError) Unwrap() error      { return e.cause }
func (*modelRequestError) Is(target error) bool { return target == ErrModelRequestFailed }
