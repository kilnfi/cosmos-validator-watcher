package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/rs/zerolog/log"
)

type Webhook struct {
	endpoint url.URL
	client   *http.Client
}

func New(endpoint url.URL) *Webhook {
	return &Webhook{
		endpoint: endpoint,
		client:   &http.Client{},
	}
}

func (w *Webhook) Send(ctx context.Context, message interface{}) error {
	body, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	log.Info().Msgf("sending webhook: %s", body)

	req, err := http.NewRequestWithContext(ctx, "POST", w.endpoint.String(), bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	retryOpts := []retry.Option{
		retry.Context(ctx),
		retry.Delay(1 * time.Second),
		retry.Attempts(3),
		retry.OnRetry(func(_ uint, err error) {
			log.Warn().Err(err).Msgf("retrying webhook on %s", w.endpoint.String())
		}),
	}

	return retry.Do(func() error {
		return w.postRequest(ctx, req)
	}, retryOpts...)
}

func (w *Webhook) postRequest(ctx context.Context, req *http.Request) error {
	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check if response is not 4xx or 5xx
	if resp.StatusCode >= 400 {
		return fmt.Errorf("unexpected response status: %s", resp.Status)
	}

	return nil
}
