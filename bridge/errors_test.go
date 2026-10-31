package bridge

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteAPIErrorProducesStructuredFields(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeAPIError(recorder, http.StatusBadRequest, "invalid_platform", "unsupported platform", map[string]any{
		"platform": "unknown",
	})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", recorder.Code)
	}
	var payload apiError
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Code != "invalid_platform" || payload.Message != "unsupported platform" {
		t.Fatalf("payload = %#v", payload)
	}
	if payload.Details["platform"] != "unknown" {
		t.Fatalf("details = %#v", payload.Details)
	}
}

func TestErrorCodeForStatus(t *testing.T) {
	cases := map[int]string{
		http.StatusBadRequest:            "invalid_request",
		http.StatusRequestEntityTooLarge: "invalid_request",
		http.StatusUnauthorized:          "unauthorized",
		http.StatusNotFound:              "not_found",
		http.StatusConflict:              "conflict",
		http.StatusBadGateway:            "upstream_error",
		http.StatusServiceUnavailable:    "service_unavailable",
		http.StatusInternalServerError:   "internal_error",
	}
	for status, want := range cases {
		if got := errorCodeForStatus(status); got != want {
			t.Fatalf("status %d code = %q, want %q", status, got, want)
		}
	}
}

func TestWriteErrorProducesStructuredResponse(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeError(recorder, httpError{status: http.StatusBadRequest, err: errors.New("invalid JSON")})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", recorder.Code)
	}
	var payload apiError
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Code != "invalid_request" || payload.Message != "invalid JSON" {
		t.Fatalf("payload = %#v", payload)
	}
}
