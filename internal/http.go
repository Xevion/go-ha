// http is used to interact with the home assistant
// REST API. Currently only used to retrieve state for
// a single entity_id
package internal

import (
	"context"
	"errors"
	"net/url"
	"time"

	"resty.dev/v3"
)

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
		SetHeader("User-Agent", "go-ha/"+currentVersion).
		SetContext(ctx)

	return &HttpClient{
		client: client,
		baseRequest: client.R().
			SetContentType("application/json").
			SetHeader("Accept", "application/json").
			SetAuthToken(token),
	}
}

// getRequest returns a new request
func (c *HttpClient) getRequest() *resty.Request {
	return c.baseRequest.Clone(c.client.Context())
}

func (c *HttpClient) GetState(entityId string) ([]byte, error) {
	resp, err := c.getRequest().Get("/states/" + entityId)

	if err != nil {
		return nil, errors.New("Error making HTTP request: " + err.Error())
	}

	if resp.StatusCode() >= 400 {
		return nil, errors.New("HTTP error: " + resp.Status() + " - " + string(resp.Bytes()))
	}

	return resp.Bytes(), nil
}

// GetStates returns the states of all entities.
func (c *HttpClient) GetStates() ([]byte, error) {
	resp, err := c.getRequest().Get("/states")

	if err != nil {
		return nil, errors.New("Error making HTTP request: " + err.Error())
	}

	if resp.StatusCode() >= 400 {
		return nil, errors.New("HTTP error: " + resp.Status() + " - " + string(resp.Bytes()))
	}

	return resp.Bytes(), nil
}
