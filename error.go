package oklog

import (
	"encoding/json"
	"github.com/pkg/errors"
)

// newMarshallableError builds an error which encodes its error message into JSON
func newMarshallableError(err error) *marshallableError {
	return &marshallableError{err}
}

// a marshallableError is an error that can be encoded into JSON
type marshallableError struct {
	err error
}

// MarshalJSON implements json.Marshaller for marshallableError
func (m *marshallableError) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.err.Error())
}

type causer interface {
	Cause() error
}

type stackTracer interface {
	StackTrace() errors.StackTrace
}

func extractStackTrace(err error) errors.StackTrace {
	var tracer stackTracer
	for {
		if st, ok := err.(stackTracer); ok {
			tracer = st
		}
		if cause, ok := err.(causer); ok {
			err = cause.Cause()
			continue
		}
		break
	}
	if tracer == nil {
		return nil
	}
	return tracer.StackTrace()
}
