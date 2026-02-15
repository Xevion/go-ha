package internal

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"resty.dev/v3"
)

var (
	// ErrUnauthorized reports a token Home Assistant refused.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrEntityNotFound reports an entity Home Assistant does not know about.
	ErrEntityNotFound = errors.New("entity not found")
	// ErrHttpStatus reports any other non-success response.
	ErrHttpStatus = errors.New("unexpected http status")
)

// statusError maps a response status onto a sentinel, so callers can match with
// errors.Is instead of matching on the message text.
func statusError(resp *resty.Response) error {
	switch resp.StatusCode() {
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrUnauthorized
	case http.StatusNotFound:
		return ErrEntityNotFound
	}
	return fmt.Errorf("%w %s", ErrHttpStatus, resp.Status())
}

type HttpClient struct {
	client      *resty.Client
	baseRequest *resty.Request
}

func NewHttpClient(ctx context.Context, baseUrl *url.URL, token string) *HttpClient {
	// Shallow copy the URL to avoid modifying the original
	u := *baseUrl
	u.Path = "/api"

	// Create resty client with configuration
	client := resty.New().
		SetBaseURL(u.String()).
		SetTimeout(30*time.Second).
		SetRetryCount(3).
		SetRetryWaitTime(1*time.Second).
		SetRetryMaxWaitTime(5*time.Second).
		AddRetryConditions(func(r *resty.Response, err error) bool {
			return err != nil || (r.StatusCode() >= 500 && r.StatusCode() != 403)
		}).
		SetHeader("User-Agent", "go-ha/"+Version).
		SetContext(ctx)

	return &HttpClient{
		client: client,
		baseRequest: client.R().
			SetContentType("application/json").
			SetHeader("Accept", "application/json").
			SetAuthToken(token),
	}
}

// getRequest returns a new request.
func (c *HttpClient) getRequest() *resty.Request {
	return c.baseRequest.Clone(c.client.Context())
}

func (c *HttpClient) GetState(entityId string) ([]byte, error) {
	resp, err := c.getRequest().Get("/states/" + entityId)

	if err != nil {
		return nil, fmt.Errorf("requesting state of %q: %w", entityId, err)
	}

	if resp.StatusCode() >= 400 {
		return nil, fmt.Errorf("requesting state of %q: %w: %s", entityId, statusError(resp), resp.Bytes())
	}

	return resp.Bytes(), nil
}

// GetStates returns the states of all entities.
func (c *HttpClient) GetStates() ([]byte, error) {
	resp, err := c.getRequest().Get("/states")

	if err != nil {
		return nil, fmt.Errorf("requesting all states: %w", err)
	}

	if resp.StatusCode() >= 400 {
		return nil, fmt.Errorf("requesting all states: %w: %s", statusError(resp), resp.Bytes())
	}

	return resp.Bytes(), nil
}
