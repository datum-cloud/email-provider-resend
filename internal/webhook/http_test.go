package webhook

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	svix "github.com/svix/svix-webhooks/go"
)

// helper to create svix webhook with deterministic secret
func newTestSvix() *svix.Webhook {
	secret := "whsec_c2VjcmV0LWtleQ==" // base64 "secret-key"
	wh, _ := svix.NewWebhook(secret)
	return wh
}

// helper to create signature headers for payload
func makeHeaders(wh *svix.Webhook, payload []byte) http.Header {
	timestamp := time.Now()
	msgID := "msg_123"
	sig, _ := wh.Sign(msgID, timestamp, payload)
	h := http.Header{}
	h.Set("svix-id", msgID)
	h.Set("svix-timestamp", strconv.FormatInt(timestamp.Unix(), 10))
	h.Set("svix-signature", sig)
	return h
}

func TestResponseHelpers(t *testing.T) {
	cases := []struct {
		name   string
		resp   Response
		expect int
	}{
		{"OK", OkResponse(), http.StatusOK},
		{"BadRequest", BadRequestResponse(), http.StatusBadRequest},
		{"MethodNotAllowed", MethodNotAllowedResponse(), http.StatusMethodNotAllowed},
		{"InternalServerError", InternalServerErrorResponse(), http.StatusInternalServerError},
		{"NotFound", NotFoundResponse(), http.StatusNotFound},
		{"Unauthorized", UnauthorizedResponse(), http.StatusUnauthorized},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.resp.HttpStatus != tc.expect {
				t.Fatalf("unexpected status, got %d want %d", tc.resp.HttpStatus, tc.expect)
			}
		})
	}
}

// test handler that records invocation
type testHandler struct {
	called bool
	resp   Response
}

func (h *testHandler) Handle(ctx context.Context, req Request) Response {
	h.called = true
	return h.resp
}

func TestServeHTTP_MethodNotAllowed(t *testing.T) {
	handler := &testHandler{resp: OkResponse()}
	wh := &Webhook{Handler: handler}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	wh.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected %d got %d", http.StatusMethodNotAllowed, rr.Code)
	}
	if allow := rr.Header().Get("Allow"); allow != http.MethodPost {
		t.Fatalf("expected Allow header %s got %s", http.MethodPost, allow)
	}
	if handler.called {
		t.Fatalf("handler should not be called for MethodNotAllowed")
	}
}

func TestServeHTTP_InvalidSignature(t *testing.T) {
	payload := []byte(`{"foo":"bar"}`)
	wh := &Webhook{Handler: &testHandler{resp: OkResponse()}, svix: newTestSvix()}

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(payload))
	// do not set required svix headers -> invalid signature
	rr := httptest.NewRecorder()

	wh.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d got %d", http.StatusUnauthorized, rr.Code)
	}
}

func TestServeHTTP_BadRequest(t *testing.T) {
	payload := []byte(`{"bad json"`)
	wh := &Webhook{Handler: &testHandler{resp: OkResponse()}, svix: newTestSvix()}

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(payload))
	req.Header = makeHeaders(wh.svix, payload)
	rr := httptest.NewRecorder()

	wh.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected %d got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestServeHTTP_Success(t *testing.T) {
	payload := []byte(`{"type":"email.sent","created_at":"2024-01-01T00:00:00Z","data":{}}`)
	handler := &testHandler{resp: OkResponse()}
	wh := &Webhook{Handler: handler, svix: newTestSvix()}

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(payload))
	req.Header = makeHeaders(wh.svix, payload)
	rr := httptest.NewRecorder()

	wh.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected %d got %d", http.StatusOK, rr.Code)
	}
	if !handler.called {
		t.Fatalf("handler was not called on success path")
	}
}
