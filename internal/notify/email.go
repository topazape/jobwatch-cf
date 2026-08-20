package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	emailEndpoint      = "https://api.cloudflare.com/client/v4/accounts/%s/email/sending/send"
	emailErrBodyLimit  = 256 // 失敗時に切り分け用として残す body 先頭バイト数
	emailSubjectPrefix = "jobwatch"
)

type Email struct {
	AccountID string
	Token     string
	From      string
	To        string
	Client    *http.Client
}

func (e *Email) client() *http.Client {
	if e.Client != nil {
		return e.Client
	}

	return &http.Client{Timeout: 15 * time.Second}
}

type emailRequest struct {
	To      string `json:"to"`
	From    string `json:"from"`
	Subject string `json:"subject"`
	Text    string `json:"text"`
}

type emailResponse struct {
	Success bool `json:"success"`
}

func (e *Email) Notify(ctx context.Context, subject, text string) error {
	body, err := json.Marshal(emailRequest{
		To:      e.To,
		From:    e.From,
		Subject: subject,
		Text:    text,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf(emailEndpoint, e.AccountID), bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+e.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client().Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close() //nolint:errcheck

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var out emailResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("decode response: %w (status=%s body=%q)", err, resp.Status, emailSnippet(raw))
	}

	if !out.Success {
		return fmt.Errorf("unexpected response: status=%s body=%q", resp.Status, emailSnippet(raw))
	}

	return nil
}

func emailSnippet(b []byte) string {
	if len(b) > emailErrBodyLimit {
		return string(b[:emailErrBodyLimit])
	}

	return string(b)
}
