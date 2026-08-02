package notify

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const ntfyBaseURL = "https://ntfy.sh/"

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

func (n *Ntfy) Notify(ctx context.Context, text string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ntfyBaseURL+n.Topic, strings.NewReader(text))
	if err != nil {
		return err
	}

	req.Header.Set("Title", "jobwatch")

	resp, err := n.client().Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %s", resp.Status)
	}

	return nil
}
