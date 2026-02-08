package httpclient

import (
	"crypto/tls"
	"net/http"
	"time"

	"github.com/go-resty/resty/v2"
)

// Client wraps resty for HTTP requests to external panels and APIs.
// Replaces PHP's CurlRequest class.
type Client struct {
	r *resty.Client
}

// New creates a new HTTP client with sensible defaults.
func New() *Client {
	r := resty.New().
		SetTimeout(30 * time.Second).
		SetRetryCount(2).
		SetRetryWaitTime(1 * time.Second).
		SetRetryMaxWaitTime(5 * time.Second)

	return &Client{r: r}
}

// WithTimeout sets a custom timeout.
func (c *Client) WithTimeout(d time.Duration) *Client {
	c.r.SetTimeout(d)
	return c
}

// WithBearerToken sets a bearer token for authentication.
func (c *Client) WithBearerToken(token string) *Client {
	c.r.SetAuthToken(token)
	return c
}

// WithCookie sets a cookie.
func (c *Client) WithCookie(name, value string) *Client {
	c.r.SetCookie(&http.Cookie{Name: name, Value: value})
	return c
}

// WithHeader sets a custom header.
func (c *Client) WithHeader(key, value string) *Client {
	c.r.SetHeader(key, value)
	return c
}

// WithInsecureSkipVerify disables TLS verification.
func (c *Client) WithInsecureSkipVerify() *Client {
	c.r.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
	return c
}

// Get sends a GET request and returns the response body.
func (c *Client) Get(url string) ([]byte, error) {
	resp, err := c.r.R().Get(url)
	if err != nil {
		return nil, err
	}
	return resp.Body(), nil
}

// Post sends a POST request with JSON body.
func (c *Client) Post(url string, body interface{}) ([]byte, error) {
	req := c.r.R().SetHeader("Content-Type", "application/json")
	if body != nil {
		req.SetBody(body)
	}
	resp, err := req.Post(url)
	if err != nil {
		return nil, err
	}
	return resp.Body(), nil
}

// PostForm sends a POST request with form data.
func (c *Client) PostForm(url string, data map[string]string) ([]byte, error) {
	resp, err := c.r.R().SetFormData(data).Post(url)
	if err != nil {
		return nil, err
	}
	return resp.Body(), nil
}

// Put sends a PUT request with JSON body.
func (c *Client) Put(url string, body interface{}) ([]byte, error) {
	resp, err := c.r.R().
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		Put(url)
	if err != nil {
		return nil, err
	}
	return resp.Body(), nil
}

// Patch sends a PATCH request with JSON body.
func (c *Client) Patch(url string, body interface{}) ([]byte, error) {
	resp, err := c.r.R().
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		Patch(url)
	if err != nil {
		return nil, err
	}
	return resp.Body(), nil
}

// Delete sends a DELETE request.
func (c *Client) Delete(url string) ([]byte, error) {
	resp, err := c.r.R().Delete(url)
	if err != nil {
		return nil, err
	}
	return resp.Body(), nil
}

// Raw returns the underlying resty client for advanced usage.
func (c *Client) Raw() *resty.Client {
	return c.r
}

// Request returns a new resty Request for chaining.
func (c *Client) Request() *resty.Request {
	return c.r.R()
}
