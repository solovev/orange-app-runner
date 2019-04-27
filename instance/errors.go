package instance

import "fmt"

type TraceeExitError struct {
	Code int
}

func (e *TraceeExitError) Error() string {
	return fmt.Sprintf("tracee exit error (code: %d)", e.Code)
}

type TracerError struct {
	Tag    string
	Code   int
	Parent error
}

func (e TracerError) Error() string {
	return fmt.Sprintf("tracer error (code: %d): \"%v\"", e.Code, e.Parent)
}

func defineTracerError(code int, parent error) *TracerError {
	return &TracerError{Code: code, Parent: parent}
}

func createTracerError(tag string, parent error) *TracerError {
	return &TracerError{Code: 1, Tag: tag, Parent: parent}
}

// = = = = = = = = = = = = = = = = = = = = = = = =
