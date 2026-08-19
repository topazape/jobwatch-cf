package notify

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sethvargo/go-retry"
)

const (
	ntfyBaseURL      = "https://ntfy.sh/"
	ntfyErrBodyLimit = 256              // 非 200 時に切り分け用として残す body 先頭バイト数
	ntfyRetryWait    = 10 * time.Second // ntfy の visitor bucket は 5s に1トークン補充される
	ntfyMaxRetries   = 2
)

type Ntfy struct {
	Topic  string
	Client *http.Client
}

func (n *Ntfy) client() *http.Client {
	if n.Client != nil {
		return n.Client
	}

	return &http.Client{Timeout: 15 * time.Second}
}

type httpError struct {
	StatusCode int
	msg        string
}

func (e *httpError) Error() string { return e.msg }

// Notify は本文を送る。429 のみリトライする。ntfy の rate limit は送信元 IP 単位で、
// Workers の egress IP は他テナントと共有されるため、自分の送信量と無関係に 429 になりうる
func (n *Ntfy) Notify(ctx context.Context, text string) error {
	client := n.client()
	b := retry.WithMaxRetries(ntfyMaxRetries, retry.NewExponential(ntfyRetryWait))

	return retry.Do(ctx, b, func(ctx context.Context) error {
		err := n.send(ctx, client, text)

		var herr *httpError
		if errors.As(err, &herr) && herr.StatusCode == http.StatusTooManyRequests {
			return retry.RetryableError(err)
		}

		return err
	},
	)
}

func (n *Ntfy) send(ctx context.Context, client *http.Client, text string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ntfyBaseURL+n.Topic, strings.NewReader(text))
	if err != nil {
		return err
	}

	req.Header.Set("Title", "jobwatch")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, ntfyErrBodyLimit))

		return &httpError{
			StatusCode: resp.StatusCode,
			msg: fmt.Sprintf("unexpected status: %s (retry-after=%q body=%q)",
				resp.Status, resp.Header.Get("Retry-After"), snippet),
		}
	}

	return nil
}
