package resend

import (
	"reflect"
	"testing"
	"time"
)

func TestParseEvent_VariousBodies(t *testing.T) {
	cases := []struct {
		name           string
		body           string
		expectedType   EmailEventType
		expectedClick  *Click
		expectedBounce *Bounce
		expectedFailed *Failed
	}{
		{
			name:         "email.sent",
			expectedType: EventTypeSent,
			body: `{
				"type": "email.sent",
				"created_at": "2024-02-22T23:41:12.126Z",
				"data": {
					"broadcast_id": "8b146471-e88e-4322-86af-016cd36fd216",
					"created_at": "2024-02-22T23:41:11.894719+00:00",
					"email_id": "56761188-7520-42d8-8898-ff6fc54ce618",
					"from": "Acme <onboarding@resend.dev>",
					"to": ["delivered@resend.dev"],
					"subject": "Sending this example",
					"tags": [{"name": "category", "value": "confirm_email"}]
				}
			}`,
		},
		{
			name:         "email.delivered",
			expectedType: EventTypeDelivered,
			body: `{
				"type": "email.delivered",
				"created_at": "2024-02-22T23:41:12.126Z",
				"data": {
					"broadcast_id": "8b146471-e88e-4322-86af-016cd36fd216",
					"created_at": "2024-02-22T23:41:11.894719+00:00",
					"email_id": "56761188-7520-42d8-8898-ff6fc54ce618",
					"from": "Acme <onboarding@resend.dev>",
					"to": ["delivered@resend.dev"],
					"subject": "Sending this example",
					"tags": [{"name": "category", "value": "confirm_email"}]
				}
			}`,
		},
		{
			name:         "email.delivery_delayed",
			expectedType: EventTypeDeliveredDelayed,
			body: `{
				"type": "email.delivery_delayed",
				"created_at": "2024-02-22T23:41:12.126Z",
				"data": {
					"broadcast_id": "8b146471-e88e-4322-86af-016cd36fd216",
					"created_at": "2024-02-22T23:41:11.894719+00:00",
					"email_id": "56761188-7520-42d8-8898-ff6fc54ce618",
					"from": "Acme <onboarding@resend.dev>",
					"to": ["delivered@resend.dev"],
					"subject": "Sending this example",
					"tags": [{"name": "category", "value": "confirm_email"}]
				}
			}`,
		},
		{
			name:         "email.complained",
			expectedType: EventTypeComplained,
			body: `{
				"type": "email.complained",
				"created_at": "2024-02-22T23:41:12.126Z",
				"data": {
					"broadcast_id": "8b146471-e88e-4322-86af-016cd36fd216",
					"created_at": "2024-02-22T23:41:11.894719+00:00",
					"email_id": "56761188-7520-42d8-8898-ff6fc54ce618",
					"from": "Acme <onboarding@resend.dev>",
					"to": ["delivered@resend.dev"],
					"subject": "Sending this example",
					"tags": [{"name": "category", "value": "confirm_email"}]
				}
			}`,
		},
		{
			name:           "email.bounced",
			expectedType:   EventTypeBounced,
			expectedBounce: &Bounce{Message: "msg", SubType: "Suppressed", Type: "Permanent"},
			body: `{
				"type": "email.bounced",
				"created_at": "2024-11-22T23:41:12.126Z",
				"data": {
					"broadcast_id": "8b146471-e88e-4322-86af-016cd36fd216",
					"created_at": "2024-11-22T23:41:11.894719+00:00",
					"email_id": "56761188-7520-42d8-8898-ff6fc54ce618",
					"from": "Acme <onboarding@resend.dev>",
					"to": ["delivered@resend.dev"],
					"subject": "Sending this example",
					"bounce": {"message": "msg", "subType": "Suppressed", "type": "Permanent"},
					"tags": [{"name": "category", "value": "confirm_email"}]
				}
			}`,
		},
		{
			name:         "email.opened",
			expectedType: EventTypeOpened,
			body: `{
				"type": "email.opened",
				"created_at": "2024-02-22T23:41:12.126Z",
				"data": {
					"broadcast_id": "8b146471-e88e-4322-86af-016cd36fd216",
					"created_at": "2024-02-22T23:41:11.894719+00:00",
					"email_id": "56761188-7520-42d8-8898-ff6fc54ce618",
					"from": "Acme <onboarding@resend.dev>",
					"to": ["delivered@resend.dev"],
					"subject": "Sending this example",
					"tags": [{"name": "category", "value": "confirm_email"}]
				}
			}`,
		},
		{
			name:         "email.clicked",
			expectedType: EventTypeClicked,
			expectedClick: func() *Click {
				ts, _ := time.Parse(time.RFC3339Nano, "2024-11-24T05:00:57.163Z")
				return &Click{IPAddress: "1.1.1.1", Link: "https://resend.com", Timestamp: ts, UserAgent: "ua"}
			}(),
			body: `{
				"type": "email.clicked",
				"created_at": "2024-11-22T23:41:12.126Z",
				"data": {
					"broadcast_id": "8b146471-e88e-4322-86af-016cd36fd216",
					"created_at": "2024-11-22T23:41:11.894719+00:00",
					"email_id": "56761188-7520-42d8-8898-ff6fc54ce618",
					"from": "Acme <onboarding@resend.dev>",
					"to": ["delivered@resend.dev"],
					"click": {"ipAddress": "1.1.1.1", "link": "https://resend.com", "timestamp": "2024-11-24T05:00:57.163Z", "userAgent": "ua"},
					"subject": "Sending this example",
					"tags": [{"name": "category", "value": "confirm_email"}]
				}
			}`,
		},
		{
			name:           "email.failed",
			expectedType:   EventTypeEmailFailed,
			expectedFailed: &Failed{Reason: "quota"},
			body: `{
				"type": "email.failed",
				"created_at": "2024-11-22T23:41:12.126Z",
				"data": {
					"broadcast_id": "8b146471-e88e-4322-86af-016cd36fd216",
					"created_at": "2024-11-22T23:41:11.894719+00:00",
					"email_id": "56761188-7520-42d8-8898-ff6fc54ce618",
					"from": "Acme <onboarding@resend.dev>",
					"to": ["delivered@resend.dev"],
					"subject": "Sending this example",
					"failed": {"reason": "quota"},
					"tags": [{"name": "category", "value": "confirm_email"}]
				}
			}`,
		},
		{
			name:         "email.scheduled",
			expectedType: EventTypeScheduled,
			body: `{
				"created_at": "2025-08-19T12:51:39.710Z",
				"data": {
					"created_at": "2025-08-19T12:51:39.600662+00:00",
					"email_id": "0cbbc092-b503-4a3e-a64d-52d3d7d597db",
					"from": "Acme <onboarding@resend.dev>",
					"subject": "hello world",
					"to": ["delivered@resend.dev"]
				},
				"type": "email.scheduled"
			}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evt, err := ParseEmailEvent([]byte(tc.body))
			if err != nil {
				t.Fatalf("ParseEvent returned error: %v", err)
			}
			if evt.Envelope.Type != tc.expectedType {
				t.Errorf("unexpected type: got %s want %s", evt.Envelope.Type, tc.expectedType)
			}
			if tc.expectedClick != nil {
				if !reflect.DeepEqual(evt.Click, tc.expectedClick) {
					t.Errorf("click payload mismatch: got %+v want %+v", evt.Click, tc.expectedClick)
				}
			}

			if tc.expectedBounce != nil {
				if !reflect.DeepEqual(evt.Bounce, tc.expectedBounce) {
					t.Errorf("bounce payload mismatch: got %+v want %+v", evt.Bounce, tc.expectedBounce)
				}
			}

			if tc.expectedFailed != nil {
				if !reflect.DeepEqual(evt.Failed, tc.expectedFailed) {
					t.Errorf("failed payload mismatch: got %+v want %+v", evt.Failed, tc.expectedFailed)
				}
			}
		})
	}
}

func TestParseEvent_Errors(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "invalid json", body: "{"},
		{name: "unknown type", body: `{"type":"email.unknown","data":{}}`},
		{name: "invalid click payload", body: `{"type":"email.clicked","data":{"click":"bad"}}`},
		{name: "invalid data section", body: `{"type":"email.sent","data":"foo"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseEmailEvent([]byte(tc.body)); err == nil {
				t.Fatalf("expected error but got nil")
			}
		})
	}
}
