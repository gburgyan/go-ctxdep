package ctxdep

import (
	"fmt"
	"reflect"
)

type DependencyError struct {
	Message        string
	ReferencedType reflect.Type
	Status         string
	SourceError    error
}

func (e *DependencyError) Error() string {
	if e.SourceError == nil {
		return fmt.Sprintf("%s: %v", e.Message, e.ReferencedType)
	} else {
		return fmt.Sprintf("%s: %v (%v)", e.Message, e.ReferencedType, e.Unwrap().Error())
	}
}

func (e *DependencyError) Unwrap() error {
	return e.SourceError
}
