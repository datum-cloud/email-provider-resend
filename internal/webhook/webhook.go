package webhook

import (
	"context"
	"net/http"
)

type Webhook struct {
	Handler  Handler
	Endpoint string
}

func NewResendWebhook() *Webhook {
	return &Webhook{
		Handler: HandlerFunc(func(ctx context.Context, req Request) Response {

			// TODO Jose: Handle the event.

			return Response{
				HttpStatus: http.StatusOK,
			}
		}),
		Endpoint: "/apis/emailnotification.k8s.io/v1/resend",
	}
}
