package bridge

import "net/http"

type apiError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func writeAPIError(response http.ResponseWriter, status int, code, message string, details map[string]any) {
	if code == "" {
		code = "internal_error"
	}
	if message == "" {
		message = http.StatusText(status)
	}
	writeJSON(response, status, apiError{
		Code: code, Message: message, Details: details,
	})
}

func errorCodeForStatus(status int) string {
	switch status {
	case http.StatusBadRequest, http.StatusRequestEntityTooLarge:
		return "invalid_request"
	case http.StatusUnauthorized:
		return "unauthorized"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusConflict:
		return "conflict"
	case http.StatusBadGateway:
		return "upstream_error"
	case http.StatusServiceUnavailable:
		return "service_unavailable"
	default:
		if status >= 500 {
			return "internal_error"
		}
		return "request_failed"
	}
}
