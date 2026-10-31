package shelf

import (
	"errors"
	"fmt"
)

// Sentinel errors; the API maps them to status codes.
var (
	ErrNotFound          = errors.New("not found")
	ErrInvalidTransition = errors.New("invalid transition")
	ErrPieceEnded        = errors.New("piece ended")
	ErrLastObservation   = errors.New("last observation")
)

// Error carries a sentinel and a message for the user.
type Error struct {
	Err error
	Msg string
}

func (e *Error) Error() string { return e.Msg }
func (e *Error) Unwrap() error { return e.Err }

func errorf(sentinel error, format string, args ...any) error {
	return &Error{Err: sentinel, Msg: fmt.Sprintf(format, args...)}
}

// Field error codes (api-contract §1.1).
const (
	CodeRequired      = "required"
	CodeInvalidEnum   = "invalid_enum"
	CodeInvalidValue  = "invalid_value"
	CodeInvalidTime   = "invalid_time"
	CodeInFuture      = "in_future"
	CodeBeforeAdded   = "before_added_at"
	CodeAfterEnded    = "after_ended_at"
	CodeOutOfRange    = "out_of_range"
	CodeTooLong       = "too_long"
	CodeDuplicate     = "duplicate"
	CodePhotoNotFound = "photo_not_found"
)

// FieldError is one invalid field.
type FieldError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ValidationError collects field errors; it becomes a 422.
type ValidationError struct {
	Fields []FieldError
}

func (v *ValidationError) Error() string {
	if len(v.Fields) == 1 {
		return "1 field is invalid"
	}
	return fmt.Sprintf("%d fields are invalid", len(v.Fields))
}

// add records a field error.
func (v *ValidationError) add(field, code, msg string) {
	v.Fields = append(v.Fields, FieldError{Field: field, Code: code, Message: msg})
}

// err returns v if it holds any errors, else nil.
func (v *ValidationError) err() error {
	if len(v.Fields) == 0 {
		return nil
	}
	return v
}

// Invalid returns a ValidationError for a single field.
func Invalid(field, code, msg string) error {
	return &ValidationError{Fields: []FieldError{{Field: field, Code: code, Message: msg}}}
}
