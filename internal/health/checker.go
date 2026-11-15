package health

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

const DefaultEndpoint = "https://api.pushover.net/1/"

type Checker struct {
	endpoint   string
	httpClient *http.Client
}

func NewChecker(endpoint string, httpClient *http.Client) *Checker {
	return &Checker{
		endpoint:   endpoint,
		httpClient: httpClient,
	}
}

func (c *Checker) Check(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint, nil)
	if err != nil {
		return fmt.Errorf("create connectivity request: %w", err)
	}
	req.Header.Set("User-Agent", "CupNotifyer-Healthcheck/1")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("external connectivity check failed: %w", err)
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))

	return nil
}
