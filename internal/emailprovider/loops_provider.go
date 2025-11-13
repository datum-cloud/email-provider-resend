package emailprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// LoopsEmail is a minimal HTTP client for Loops contacts API.
type LoopsEmail struct {
	token      string
	baseURL    string
	httpClient *http.Client
}

// NewLoopsEmail creates a new LoopsEmail client.
// token is the Loops API token (without the "Bearer " prefix).
// baseURL is optional; if empty, defaults to "https://app.loops.so".
func NewLoopsEmail(token string) *LoopsEmail {
	baseURL := "https://app.loops.so"

	return &LoopsEmail{
		token:   token,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

type loopsCreateOrUpdatePayload struct {
	Email        string                 `json:"email,omitempty"`
	FirstName    string                 `json:"firstName,omitempty"`
	LastName     string                 `json:"lastName,omitempty"`
	Source       string                 `json:"source,omitempty"`
	Subscribed   bool                   `json:"subscribed"`
	UserGroup    string                 `json:"userGroup,omitempty"`
	UserID       string                 `json:"userId,omitempty"`
	MailingLists map[string]interface{} `json:"mailingLists,omitempty"`
}

type loopsDeletePayload struct {
	Email  string `json:"email,omitempty"`
	UserID string `json:"userId,omitempty"`
}

// LoopsCreateResponse matches the Loops create contact response shape.
type LoopsCreateResponse struct {
	Success bool   `json:"success"`
	ID      string `json:"id"`
}

// LoopsContact represents a Loops contact in API responses.
type LoopsContact struct {
	ID           string          `json:"id"`
	Email        string          `json:"email"`
	FirstName    string          `json:"firstName"`
	LastName     string          `json:"lastName"`
	Source       string          `json:"source"`
	Subscribed   bool            `json:"subscribed"`
	UserGroup    string          `json:"userGroup"`
	UserID       string          `json:"userId"`
	MailingLists map[string]bool `json:"mailingLists"`
	OptInStatus  string          `json:"optInStatus"`
}

// LoopsHTTPResponse contains the raw HTTP response for troubleshooting.
type LoopsHTTPResponse struct {
	StatusCode int
	Body       []byte
}

// LoopsDeleteResponse matches the Loops delete contact response shape.
//
//	{
//	  "success": true,
//	  "message": "Contact deleted."
//	}
type LoopsDeleteResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// LoopsMailingList represents a single mailing list subscription.
type LoopsMailingList struct {
	ID         string
	Subscribed bool
}

func mailingListsSliceToPayload(entries []LoopsMailingList) map[string]interface{} {
	if entries == nil {
		return nil
	}
	out := make(map[string]interface{}, len(entries))
	for _, e := range entries {
		out[e.ID] = e.Subscribed
	}
	return out
}

func preferredContactName(email, userID string) string {
	if userID != "" {
		return userID
	}
	return email
}

func (l *LoopsEmail) do(ctx context.Context, method, path string, body any) (LoopsHTTPResponse, error) {
	var (
		reqBody io.Reader
	)
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return LoopsHTTPResponse{}, fmt.Errorf("marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(buf)
	}

	u, err := url.Parse(l.baseURL)
	if err != nil {
		return LoopsHTTPResponse{}, fmt.Errorf("parse baseURL: %w", err)
	}
	u.Path = path

	req, err := http.NewRequestWithContext(ctx, method, u.String(), reqBody)
	if err != nil {
		return LoopsHTTPResponse{}, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+l.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	res, err := l.httpClient.Do(req)
	if err != nil {
		return LoopsHTTPResponse{}, fmt.Errorf("do request: %w", err)
	}
	defer res.Body.Close()

	respBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return LoopsHTTPResponse{}, fmt.Errorf("read response body: %w", err)
	}

	return LoopsHTTPResponse{
		StatusCode: res.StatusCode,
		Body:       respBytes,
	}, nil
}

// CreateContact creates a Loops contact using Contact.Spec fields.
// It ALWAYS sets userId from the provided userID parameter.
func (l *LoopsEmail) CreateContact(ctx context.Context, email, firstName, lastName, userID string) (LoopsCreateResponse, error) {
	payload := loopsCreateOrUpdatePayload{
		Email:        email,
		FirstName:    firstName,
		LastName:     lastName,
		Source:       "milo-k8s-controller",
		Subscribed:   true,
		UserGroup:    "",
		UserID:       userID,
		MailingLists: map[string]interface{}{},
	}
	httpResp, err := l.do(ctx, http.MethodPost, "/api/v1/contacts/create", payload)
	if err != nil {
		return LoopsCreateResponse{}, err
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		if httpResp.StatusCode == http.StatusConflict {
			return LoopsCreateResponse{}, errors.NewConflict(
				schema.GroupResource{Group: "loops", Resource: "contacts"},
				preferredContactName(email, userID),
				fmt.Errorf("loops create conflict: %s", string(httpResp.Body)),
			)
		}
		return LoopsCreateResponse{}, fmt.Errorf("loops create contact failed: status=%d body=%s", httpResp.StatusCode, string(httpResp.Body))
	}
	var out LoopsCreateResponse
	if err := json.Unmarshal(httpResp.Body, &out); err != nil {
		return LoopsCreateResponse{}, fmt.Errorf("parse loops create contact response: %w", err)
	}
	return out, nil
}

// UpdateContact updates a Loops contact using Contact.Spec fields.
// It ALWAYS sets userId from the provided userID parameter.
func (l *LoopsEmail) UpdateContact(ctx context.Context, email, firstName, lastName, userID string, mailingLists []LoopsMailingList) ([]LoopsContact, error) {
	payload := loopsCreateOrUpdatePayload{
		Email:        email,
		FirstName:    firstName,
		LastName:     lastName,
		Source:       "milo-k8s-controller",
		Subscribed:   true,
		UserGroup:    "",
		UserID:       userID,
		MailingLists: mailingListsSliceToPayload(mailingLists),
	}
	httpResp, err := l.do(ctx, http.MethodPut, "/api/v1/contacts/update", payload)
	if err != nil {
		return nil, err
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		if httpResp.StatusCode == http.StatusConflict {
			return nil, errors.NewConflict(
				schema.GroupResource{Group: "loops", Resource: "contacts"},
				preferredContactName(email, userID),
				fmt.Errorf("loops update conflict: %s", string(httpResp.Body)),
			)
		}
		if httpResp.StatusCode == http.StatusBadRequest {
			return nil, errors.NewBadRequest(fmt.Sprintf("loops update bad request: %s", string(httpResp.Body)))
		}
		return nil, fmt.Errorf("loops update contact failed: status=%d body=%s", httpResp.StatusCode, string(httpResp.Body))
	}
	var out []LoopsContact
	body := httpResp.Body
	if len(body) == 0 {
		return nil, fmt.Errorf("empty loops update contact response")
	}
	switch body[0] {
	case '[':
		if err := json.Unmarshal(body, &out); err != nil {
			return nil, fmt.Errorf("parse loops update contact response (array): %w", err)
		}
	case '{':
		var single LoopsContact
		if err := json.Unmarshal(body, &single); err != nil {
			return nil, fmt.Errorf("parse loops update contact response (object): %w", err)
		}
		out = []LoopsContact{single}
	default:
		return nil, fmt.Errorf("unexpected loops update contact response prefix: %q", string(body[:1]))
	}
	return out, nil
}

// UpdateContactMailingLists updates ONLY the mailing lists for a contact identified by userId.
// Other fields are left untouched. This is useful when you want to subscribe/unsubscribe from lists
// without modifying core profile fields.
func (l *LoopsEmail) UpdateContactMailingLists(ctx context.Context, userID string, mailingLists []LoopsMailingList) ([]LoopsContact, error) {
	payload := loopsCreateOrUpdatePayload{
		UserID:       userID,
		Subscribed:   true,
		MailingLists: mailingListsSliceToPayload(mailingLists),
	}
	httpResp, err := l.do(ctx, http.MethodPut, "/api/v1/contacts/update", payload)
	if err != nil {
		return nil, err
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		if httpResp.StatusCode == http.StatusConflict {
			return nil, errors.NewConflict(
				schema.GroupResource{Group: "loops", Resource: "contacts"},
				preferredContactName("", userID),
				fmt.Errorf("loops update (mailing lists) conflict: %s", string(httpResp.Body)),
			)
		}
		if httpResp.StatusCode == http.StatusBadRequest {
			return nil, errors.NewBadRequest(fmt.Sprintf("loops update (mailing lists) bad request: %s", string(httpResp.Body)))
		}
		return nil, fmt.Errorf("loops update (mailing lists) failed: status=%d body=%s", httpResp.StatusCode, string(httpResp.Body))
	}
	var out []LoopsContact
	body := httpResp.Body
	if len(body) == 0 {
		return nil, fmt.Errorf("empty loops update (mailing lists) response")
	}
	switch body[0] {
	case '[':
		if err := json.Unmarshal(body, &out); err != nil {
			return nil, fmt.Errorf("parse loops update (mailing lists) response (array): %w", err)
		}
	case '{':
		var single LoopsContact
		if err := json.Unmarshal(body, &single); err != nil {
			return nil, fmt.Errorf("parse loops update (mailing lists) response (object): %w", err)
		}
		out = []LoopsContact{single}
	default:
		return nil, fmt.Errorf("unexpected loops update (mailing lists) response prefix: %q", string(body[:1]))
	}
	return out, nil
}

// DeleteContact deletes a Loops contact. It ALWAYS uses the provided userID parameter.
// Email is also sent when available to match the sample payload.
func (l *LoopsEmail) DeleteContact(ctx context.Context, userID string) (LoopsDeleteResponse, error) {
	payload := loopsDeletePayload{
		UserID: userID,
	}
	httpResp, err := l.do(ctx, http.MethodPost, "/api/v1/contacts/delete", payload)
	if err != nil {
		return LoopsDeleteResponse{}, err
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		if httpResp.StatusCode == http.StatusNotFound {
			return LoopsDeleteResponse{}, errors.NewNotFound(
				schema.GroupResource{Group: "loops", Resource: "contacts"},
				preferredContactName("", userID),
			)
		}
		return LoopsDeleteResponse{}, fmt.Errorf("loops delete contact failed: status=%d body=%s", httpResp.StatusCode, string(httpResp.Body))
	}
	var out LoopsDeleteResponse
	if err := json.Unmarshal(httpResp.Body, &out); err != nil {
		return LoopsDeleteResponse{}, fmt.Errorf("parse loops delete contact response: %w", err)
	}
	return out, nil
}

// FindContactByUserID finds Loops contacts by userId and returns a typed list.
func (l *LoopsEmail) FindContactByUserID(ctx context.Context, userID string) ([]LoopsContact, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("userID must not be empty")
	}
	u, err := url.Parse(l.baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse baseURL: %w", err)
	}
	u.Path = "/api/v1/contacts/find"
	q := u.Query()
	q.Set("userId", userID)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+l.token)

	res, err := l.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer res.Body.Close()

	respBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("loops find contact failed: status=%d body=%s", res.StatusCode, string(respBytes))
	}

	var out []LoopsContact
	if err := json.Unmarshal(respBytes, &out); err != nil {
		return nil, fmt.Errorf("parse loops find contact response: %w", err)
	}
	return out, nil
}
