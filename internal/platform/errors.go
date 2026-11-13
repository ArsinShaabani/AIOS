package platform

import (
	"errors"
	"fmt"
	"net/http"
)

// Kind classifies an error for transport mapping (HTTP status, MQTT error
// codes, CLI exit codes) without leaking implementation details.
type Kind string

const (
	KindInvalid      Kind = "invalid_argument"
	KindNotFound     Kind = "not_found"
	KindConflict     Kind = "conflict"
	KindUnauthorized Kind = "unauthorized"
	KindForbidden    Kind = "forbidden"
	KindRateLimited  Kind = "rate_limited"
	KindUnavailable  Kind = "unavailable"
	KindTimeout      Kind = "timeout"
	KindInternal     Kind = "internal"
)

// Error is the single error type travelling across layer boundaries. Callers
// use errors.Is/As with the exported helpers in this file.
type Error struct {
	Kind    Kind              `json:"-"`
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Meta    map[string]string `json:"meta,omitempty"`
	Err     error             `json:"-"`
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.Err }

// HTTPStatus maps the error kind onto a status code.
func (e *Error) HTTPStatus() int {
	switch e.Kind {
	case KindInvalid:
		return http.StatusBadRequest
	case KindNotFound:
		return http.StatusNotFound
	case KindConflict:
		return http.StatusConflict
	case KindUnauthorized:
		return http.StatusUnauthorized
	case KindForbidden:
		return http.StatusForbidden
	case KindRateLimited:
		return http.StatusTooManyRequests
	case KindUnavailable:
		return http.StatusServiceUnavailable
	case KindTimeout:
		return http.StatusGatewayTimeout
	default:
		return http.StatusInternalServerError
	}
}

// NewError builds a typed error.
func NewError(kind Kind, code, message string) *Error {
	return &Error{Kind: kind, Code: code, Message: message}
}

// Errorf builds a typed error with a formatted message.
func Errorf(kind Kind, code, format string, args ...any) *Error {
	return &Error{Kind: kind, Code: code, Message: fmt.Sprintf(format, args...)}
}

// Wrap attaches a cause to a typed error.
func Wrap(kind Kind, code, message string, err error) *Error {
	return &Error{Kind: kind, Code: code, Message: message, Err: err}
}

// Common constructors -------------------------------------------------------

func Invalid(code, message string) *Error      { return NewError(KindInvalid, code, message) }
func NotFound(code, message string) *Error     { return NewError(KindNotFound, code, message) }
func Conflict(code, message string) *Error     { return NewError(KindConflict, code, message) }
func Unauthorized(code, message string) *Error { return NewError(KindUnauthorized, code, message) }
func Forbidden(code, message string) *Error    { return NewError(KindForbidden, code, message) }
func RateLimited(code, message string) *Error  { return NewError(KindRateLimited, code, message) }
func Unavailable(code, message string) *Error  { return NewError(KindUnavailable, code, message) }
func Internal(code, message string) *Error     { return NewError(KindInternal, code, message) }

// WrapInternal wraps an unexpected error without exposing it to clients.
func WrapInternal(err error) *Error {
	return &Error{Kind: KindInternal, Code: "internal_error", Message: "internal error", Err: err}
}

// AsError normalises any error into *Error.
func AsError(err error) *Error {
	if err == nil {
		return nil
	}
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr
	}
	return WrapInternal(err)
}

// IsNotFound reports whether err is a not-found error.
func IsNotFound(err error) bool { return kindOf(err) == KindNotFound }

// IsConflict reports whether err is a conflict error.
func IsConflict(err error) bool { return kindOf(err) == KindConflict }

// IsUnauthorized reports whether err is an authentication error.
func IsUnauthorized(err error) bool { return kindOf(err) == KindUnauthorized }

// IsForbidden reports whether err is an authorisation error.
func IsForbidden(err error) bool { return kindOf(err) == KindForbidden }

// IsInvalid reports whether err is a validation error.
func IsInvalid(err error) bool { return kindOf(err) == KindInvalid }

// IsUnavailable reports whether the dependency is temporarily unusable.
func IsUnavailable(err error) bool { return kindOf(err) == KindUnavailable }

func kindOf(err error) Kind {
	if err == nil {
		return ""
	}
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr.Kind
	}
	return KindInternal
}
