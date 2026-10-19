package main

import (
	"context"
	"errors"
	"net"
	"net/http"
)

// The native host transports only known codes, never upstream messages or
// reference audio. Keep service failure categories separate from HTTP 502.
type speechServiceError struct{ kind, message string }

func (e *speechServiceError) Error() string { return e.message }
func speechFailure(kind, message string) error {
	return &speechServiceError{kind: kind, message: message}
}
func speechConnectionFailure(err error, message string) error {
	var network net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &network) && network.Timeout()) {
		return speechFailure("timeout", message)
	}
	return speechFailure("network", message)
}
func writeSpeechFailure(w http.ResponseWriter, stage string, err error) {
	kind := "failed"
	var service *speechServiceError
	if errors.As(err, &service) {
		kind = service.kind
	}
	if errors.Is(err, errVoiceProfile) {
		kind = "voice"
	}
	writeProtocolError(w, 502, "speech_"+stage+"_"+kind, err.Error())
}
