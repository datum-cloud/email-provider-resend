package webhook

import (
	"context"
	"io"
	"net/http"

	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"go.miloapis.com/email-provider-resend/internal/resend"
)

type Request struct {
	Event resend.ParsedEvent
}

type Response struct {
	HttpStatus int `json:"HttpStatus"`
}

type HandlerFunc func(context.Context, Request) Response

func (f HandlerFunc) Handle(ctx context.Context, req Request) Response {
	return f(ctx, req)
}

type Handler interface {
	Handle(context.Context, Request) Response
}

func (wh *Webhook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log := logf.FromContext(r.Context()).WithName("resend-http-webhook")
	log.Info("Handling request", "method", r.Method, "remoteAddr", r.RemoteAddr)

	// panic recovery
	defer func() {
		if r := recover(); r != nil {
			log.Error(nil, "Panic in webhook handler", "panic", r)
			wh.writeResponse(w, InternalServerErrorResponse())
		}
	}()

	if r.Method != http.MethodPost {
		log.Error(nil, "Method not allowed", "method", r.Method)
		w.Header().Set("Allow", http.MethodPost)
		wh.writeResponse(w, MethodNotAllowedResponse())
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Error(err, "Failed to read request body")
		wh.writeResponse(w, InternalServerErrorResponse())
		return
	}
	defer func() {
		if err := r.Body.Close(); err != nil {
			log.Error(err, "Failed to close request body")
		}
	}()

	// Verify the signature of the request using svix
	if err := wh.svix.Verify(body, r.Header); err != nil {
		log.Error(err, "Failed to verify request")
		wh.writeResponse(w, UnauthorizedResponse())
		return
	}

	emailEvent, err := resend.ParseEmailEvent(body)
	if err != nil {
		log.Error(err, "Failed to parse email event")
		wh.writeResponse(w, BadRequestResponse())
		return
	}

	response := wh.Handler.Handle(r.Context(), Request{
		Event: *emailEvent,
	})

	wh.writeResponse(w, response)
}

func (wh *Webhook) writeResponse(w http.ResponseWriter, response Response) {
	w.WriteHeader(response.HttpStatus)
}
