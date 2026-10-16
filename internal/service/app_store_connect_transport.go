package service

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	ascResponseHeaderTimeout = 6 * time.Second
	ascMaxGETAttempts        = 3
)

type ascRetryTransport struct {
	base  http.RoundTripper
	asc   http.RoundTripper
	sleep func(context.Context, time.Duration) error
}

func init() {
	base := http.DefaultTransport
	http.DefaultTransport = newASCRetryTransport(base)
}

func newASCRetryTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	asc := base
	if transport, ok := base.(*http.Transport); ok {
		clone := transport.Clone()
		clone.ResponseHeaderTimeout = ascResponseHeaderTimeout
		asc = clone
	}
	return &ascRetryTransport{
		base:  base,
		asc:   asc,
		sleep: sleepWithContext,
	}
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func isASCRequest(req *http.Request) bool {
	if req == nil || req.URL == nil {
		return false
	}
	return strings.EqualFold(req.URL.Hostname(), "api.appstoreconnect.apple.com")
}

func ascRetryableStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func ascTransientTransportError(ctx context.Context, err error) bool {
	if err == nil || ctx == nil || ctx.Err() != nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	message := strings.ToLower(err.Error())
	for _, fragment := range []string{
		"http2",
		"server closed idle connection",
		"connection reset",
		"broken pipe",
		"unexpected eof",
		"timeout awaiting response headers",
	} {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}

func (t *ascRetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || req.Method != http.MethodGet || !isASCRequest(req) {
		return t.base.RoundTrip(req)
	}

	for attempt := 0; attempt < ascMaxGETAttempts; attempt++ {
		if err := req.Context().Err(); err != nil {
			return nil, err
		}

		attemptReq := req.Clone(req.Context())
		resp, err := t.asc.RoundTrip(attemptReq)
		if err == nil && !ascRetryableStatus(resp.StatusCode) {
			return resp, nil
		}

		lastAttempt := attempt == ascMaxGETAttempts-1
		if err != nil {
			if lastAttempt || !ascTransientTransportError(req.Context(), err) {
				return nil, err
			}
		} else {
			if lastAttempt {
				return resp, nil
			}
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 8<<10))
			_ = resp.Body.Close()
		}

		delay := time.Duration(attempt+1) * 250 * time.Millisecond
		if err := t.sleep(req.Context(), delay); err != nil {
			return nil, err
		}
	}

	return nil, context.DeadlineExceeded
}
