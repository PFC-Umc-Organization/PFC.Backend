package apperr

import "errors"

type Kind int

const (
	KindInvalid Kind = iota + 1
	KindUnauthorized
	KindForbidden
	KindNotFound
	KindConflict
	KindTooManyRequests
	KindUnavailable
	KindInternal
)

// Error carries the message shown to the user; Err, when set, is the cause
// that gets logged.
type Error struct {
	Kind    Kind
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.Err }

func Invalid(msg string) error         { return &Error{Kind: KindInvalid, Message: msg} }
func Unauthorized(msg string) error    { return &Error{Kind: KindUnauthorized, Message: msg} }
func Forbidden(msg string) error       { return &Error{Kind: KindForbidden, Message: msg} }
func NotFound(msg string) error        { return &Error{Kind: KindNotFound, Message: msg} }
func Conflict(msg string) error        { return &Error{Kind: KindConflict, Message: msg} }
func TooManyRequests(msg string) error { return &Error{Kind: KindTooManyRequests, Message: msg} }

func Unavailable(msg string, err error) error {
	return &Error{Kind: KindUnavailable, Message: msg, Err: err}
}

func Internal(msg string, err error) error {
	return &Error{Kind: KindInternal, Message: msg, Err: err}
}

func As(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}
