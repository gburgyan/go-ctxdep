package ctxdep

import (
	"fmt"
	"reflect"
)

// DependencyError is the standard error type returned from the context dependency library.
type DependencyError struct {
	// Message is the error message that describes what when wrong.
	Message string

	// ReferencedType is the type that was in the process of resolution when the error occurred.
	ReferencedType reflect.Type

	// Status is the output of `Status(ctx)` at the time of the error. This is captured but
	// not written out with `Error()` to prevent overly long error messages.
	Status string

	// SourceError captures the underlying cause of the error, if any.
	SourceError error
}

// Error returns a string form of the error. Note that the Status is not written out to
// be more succinct for logging.
func (e *DependencyError) Error() string {
	if e.SourceError == nil {
		return fmt.Sprintf("%s: %v", e.Message, e.ReferencedType)
	} else {
		return fmt.Sprintf("%s: %v (%v)", e.Message, e.ReferencedType, e.Unwrap().Error())
	}
}

// Unwrap returns the source of the error, or nil otherwise.
func (e *DependencyError) Unwrap() error {
	return e.SourceError
}
