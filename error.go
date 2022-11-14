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

func (d *DependencyError) Error() string {
	if d.SourceError == nil {
		return fmt.Sprintf("%s: %v", d.Message, d.ReferencedType)
	} else {
		return fmt.Sprintf("%s: %v (%v)", d.Message, d.ReferencedType, d.SourceError.Error())
	}
}

func (d *DependencyError) Unwrap() error {
	return d.SourceError
}
