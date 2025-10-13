package resend

import (
	"reflect"
	"testing"
	"time"
)

func TestParseContactEvent_VariousBodies(t *testing.T) {
	// Helper to parse RFC3339 times used in the example bodies.
	mustTime := func(s string) ResendTime {
		ts, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
			t.Fatalf("failed to parse test time: %v", err)
		}
		return ResendTime{Time: ts}
	}

	cases := []struct {
		name            string
		body            string
		expectedType    ContactEventType
		expectedContact ContactBase
	}{
		{
			name:         "contact.created",
			expectedType: ContactCreated,
			expectedContact: ContactBase{
				AudienceID:   "0fcb6aea-97dd-4ba5-acf1-48321c183a41",
				CreatedAt:    mustTime("2025-09-21T00:53:54.797Z"),
				Email:        "steve@woz.com",
				FirstName:    "Steve",
				ID:           "4a5fa570-8140-4baf-a190-2361c2352905",
				LastName:     "Wozniak",
				Unsubscribed: false,
				UpdatedAt:    mustTime("2025-10-01T17:38:42.968Z"),
			},
			body: `{
  "created_at": "2025-10-01T17:38:42.986Z",
  "data": {
    "audience_id": "0fcb6aea-97dd-4ba5-acf1-48321c183a41",
    "created_at": "2025-09-21T00:53:54.797Z",
    "email": "steve@woz.com",
    "first_name": "Steve",
    "id": "4a5fa570-8140-4baf-a190-2361c2352905",
    "last_name": "Wozniak",
    "unsubscribed": false,
    "updated_at": "2025-10-01T17:38:42.968Z"
  },
  "type": "contact.created"
}`,
		},
		{
			name:         "contact.updated",
			expectedType: ContactUpdated,
			expectedContact: ContactBase{
				AudienceID:   "0fcb6aea-97dd-4ba5-acf1-48321c183a41",
				CreatedAt:    mustTime("2025-09-21T00:53:54.797Z"),
				Email:        "steve@woz.com",
				FirstName:    "Steve",
				ID:           "4a5fa570-8140-4baf-a190-2361c2352905",
				LastName:     "Wozniak",
				Unsubscribed: true,
				UpdatedAt:    mustTime("2025-10-01T17:49:51.402Z"),
			},
			body: `{
  "created_at": "2025-10-01T17:49:51.411Z",
  "data": {
    "audience_id": "0fcb6aea-97dd-4ba5-acf1-48321c183a41",
    "created_at": "2025-09-21T00:53:54.797Z",
    "email": "steve@woz.com",
    "first_name": "Steve",
    "id": "4a5fa570-8140-4baf-a190-2361c2352905",
    "last_name": "Wozniak",
    "unsubscribed": true,
    "updated_at": "2025-10-01T17:49:51.402Z"
  },
  "type": "contact.updated"
}`,
		},
		{
			name:         "contact.deleted",
			expectedType: ContactDeleted,
			expectedContact: ContactBase{
				AudienceID:   "0fcb6aea-97dd-4ba5-acf1-48321c183a41",
				CreatedAt:    mustTime("2025-09-21T00:53:54.797Z"),
				Email:        "steve@woz.com",
				FirstName:    "Steve",
				ID:           "4a5fa570-8140-4baf-a190-2361c2352905",
				LastName:     "Wozniak",
				Unsubscribed: true,
				UpdatedAt:    mustTime("2025-10-01T17:50:24.840Z"),
			},
			body: `{
  "created_at": "2025-10-01T18:02:02.347Z",
  "data": {
    "audience_id": "0fcb6aea-97dd-4ba5-acf1-48321c183a41",
    "created_at": "2025-09-21T00:53:54.797Z",
    "email": "steve@woz.com",
    "first_name": "Steve",
    "id": "4a5fa570-8140-4baf-a190-2361c2352905",
    "last_name": "Wozniak",
    "unsubscribed": true,
    "updated_at": "2025-10-01T17:50:24.840Z"
  },
  "type": "contact.deleted"
}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evt, err := ParseContactEvent([]byte(tc.body))
			if err != nil {
				t.Fatalf("ParseContactEvent returned error: %v", err)
			}
			if evt.Envelope.Type != tc.expectedType {
				t.Errorf("unexpected type: got %s want %s", evt.Envelope.Type, tc.expectedType)
			}

			// Compare decoded contact payload ignoring time zone differences by using DeepEqual.
			if !reflect.DeepEqual(evt.Contact, tc.expectedContact) {
				t.Errorf("contact payload mismatch: got %+v want %+v", evt.Contact, tc.expectedContact)
			}
		})
	}
}

func TestParseContactEvent_Errors(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "invalid jskon", body: "{"},
		{name: "unknown type", body: `{"type":"contact.unknown","data":{}}`},
		{name: "invalid data section", body: `{"type":"contact.created","data":"foo"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseContactEvent([]byte(tc.body)); err == nil {
				t.Fatalf("expected error but got nil")
			}
		})
	}
}
