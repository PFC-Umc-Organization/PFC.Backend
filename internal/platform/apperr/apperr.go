package apperr

import "errors"

// ---------------------------------------------------------------------------
// Modelo
// ---------------------------------------------------------------------------

// Enumeração
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

// Erro
type Error struct {
	Kind    Kind
	Message string
	Err     error
}

// Mensagem
func (e *Error) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

// Desembrulho
func (e *Error) Unwrap() error { return e.Err }

// ---------------------------------------------------------------------------
// Construtores
// ---------------------------------------------------------------------------

// Erro
func Invalid(msg string) error { return &Error{Kind: KindInvalid, Message: msg} }

// Erro
func Unauthorized(msg string) error { return &Error{Kind: KindUnauthorized, Message: msg} }

// Erro
func Forbidden(msg string) error { return &Error{Kind: KindForbidden, Message: msg} }

// Erro
func NotFound(msg string) error { return &Error{Kind: KindNotFound, Message: msg} }

// Erro
func Conflict(msg string) error { return &Error{Kind: KindConflict, Message: msg} }

// Erro
func TooManyRequests(msg string) error { return &Error{Kind: KindTooManyRequests, Message: msg} }

// Erro
func Unavailable(msg string, err error) error {
	return &Error{Kind: KindUnavailable, Message: msg, Err: err}
}

// Erro
func Internal(msg string, err error) error {
	return &Error{Kind: KindInternal, Message: msg, Err: err}
}

// Conversão
func As(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}
