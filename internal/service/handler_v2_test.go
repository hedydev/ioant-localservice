package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestHandlerV2BuildsWithoutRouteConflict(t *testing.T) {
	a := &App{
		token: strings.Repeat("a", 32),
		static: fstest.MapFS{
			"index.html": &fstest.MapFile{Data: []byte("ok")},
		},
	}

	h := a.HandlerV2()
	if h == nil {
		t.Fatal("HandlerV2 returned nil")
	}
}

func TestHandlerV2SeparatesEnrollmentCallbackAndAppleRegistration(t *testing.T) {
	a := &App{
		token: strings.Repeat("a", 32),
		static: fstest.MapFS{
			"index.html": &fstest.MapFile{Data: []byte("ok")},
		},
	}
	h := a.HandlerV2()

	callback := httptest.NewRequest(http.MethodPost, "/api/devices/callback/test-challenge", nil)
	callbackRecorder := httptest.NewRecorder()
	h.ServeHTTP(callbackRecorder, callback)
	if callbackRecorder.Code != http.StatusGone {
		t.Fatalf("callback status = %d, want %d", callbackRecorder.Code, http.StatusGone)
	}

	registration := httptest.NewRequest(http.MethodPost, "/api/apple-devices/00008101-000871A22292001E/register", nil)
	registrationRecorder := httptest.NewRecorder()
	h.ServeHTTP(registrationRecorder, registration)
	if registrationRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("registration status = %d, want %d", registrationRecorder.Code, http.StatusUnauthorized)
	}
}
