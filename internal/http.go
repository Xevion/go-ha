// http is used to interact with the home assistant
// REST API. Currently only used to retrieve state for
// a single entity_id
package internal

import (
	"errors"
	"net/url"
	"time"

	"resty.dev/v3"
)

type HttpClient struct {
	client *resty.Client
}

func NewHttpClient(url *url.URL, token string) *HttpClient {
	// Shallow copy the URL to avoid modifying the original
	u := *url
	u.Path = "/api"
	if u.Scheme == "ws" {
		u.Scheme = "http"
	}
	if u.Scheme == "wss" {
		u.Scheme = "https"
	}

	// Create resty client with configuration
	client := resty.New().
		SetBaseURL(u.String()).
		SetHeader("Authorization", "Bearer "+token).
		SetTimeout(30 * time.Second).
		SetRetryCount(3).
		SetRetryWaitTime(1 * time.Second).
		SetRetryMaxWaitTime(5 * time.Second).
		AddRetryConditions(func(r *resty.Response, err error) bool {
			return err != nil || r.StatusCode() >= 500
		})

	return &HttpClient{
		client: client,
	}
}

func (c *HttpClient) GetState(entityId string) ([]byte, error) {
	resp, err := c.client.R().
		Get("/states/" + entityId)

	if err != nil {
		return nil, errors.New("Error making HTTP request: " + err.Error())
	}

	if resp.StatusCode() >= 400 {
		return nil, errors.New("HTTP error: " + resp.Status() + " - " + string(resp.Bytes()))
	}

	return resp.Bytes(), nil
}

func (c *HttpClient) States() ([]byte, error) {
	resp, err := c.client.R().
		Get("/states")

	if err != nil {
		return nil, errors.New("Error making HTTP request: " + err.Error())
	}

	if resp.StatusCode() >= 400 {
		return nil, errors.New("HTTP error: " + resp.Status() + " - " + string(resp.Bytes()))
	}

	return resp.Bytes(), nil
}
