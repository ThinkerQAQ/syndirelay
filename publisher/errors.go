package publisher

import (
	"fmt"
	"strings"
)

type ErrorKind string

const (
	ErrAuthExpired        ErrorKind = "auth-expired"
	ErrCSRF               ErrorKind = "csrf"
	ErrRateLimited        ErrorKind = "rate-limited"
	ErrRemoteDraftMissing ErrorKind = "remote-draft-not-found"
	ErrPermission         ErrorKind = "permission"
	ErrValidation         ErrorKind = "validation"
	ErrUpload             ErrorKind = "upload"
	ErrUpstream           ErrorKind = "upstream"
	ErrTimeout            ErrorKind = "timeout"
	ErrNotImplemented     ErrorKind = "not-implemented"
)

type PlatformError struct {
	Kind       ErrorKind
	Platform   string
	Operation  string
	StatusCode int
	Message    string
	Retryable  bool
}

func (e *PlatformError) Error() string {
	if e == nil {
		return ""
	}
	prefix := e.Platform
	if e.Operation != "" {
		if prefix != "" {
			prefix += " "
		}
		prefix += e.Operation
	}
	if prefix == "" {
		return e.Message
	}
	if e.Message == "" {
		return prefix + " failed"
	}
	return fmt.Sprintf("%s: %s", prefix, e.Message)
}

func platformError(kind ErrorKind, platform, operation string, status int, message string, retryable bool) error {
	return &PlatformError{
		Kind: kind, Platform: platform, Operation: operation,
		StatusCode: status, Message: strings.TrimSpace(message), Retryable: retryable,
	}
}

func IsKind(err error, kind ErrorKind) bool {
	for err != nil {
		typed, ok := err.(*PlatformError)
		if ok {
			return typed.Kind == kind
		}
		type unwrapper interface{ Unwrap() error }
		value, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = value.Unwrap()
	}
	return false
}
