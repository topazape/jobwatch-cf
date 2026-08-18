package fetcher

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/sethvargo/go-retry"
)

const (
	nflxBaseURL      = "https://explore.jobs.netflix.net/api/apply/v2/jobs"
	nflxPageSize     = 10 // num の実効上限(2026-07 実測)
	nflxMaxJobs      = 2000
	nflxPageSleep    = 2 * time.Second
	nflxErrBodyLimit = 256 // 非 200 時に切り分け用として残す body 先頭バイト数
	nflxRetryWait    = 10 * time.Second
	nflxMaxRetries   = 2
	nflxUserAgent    = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36"
)

// Netflix は Netflix 採用サイト(Eightfold SmartApply API)の Fetcher。
// 検索条件は渡さず全件取得する。Eightfold の query は関連度マッチングで非決定的
type Netflix struct {
	Client *http.Client // nil ならデフォルト(timeout 15s)
}

func (n *Netflix) Slug() string { return "nflx" }

type nflxResponse struct {
	Count     int              `json:"count"`
	Positions []jsontext.Value `json:"positions"`
}

type nflxPosition struct {
	ID         int64    `json:"id"`
	Name       string   `json:"name"`
	Location   string   `json:"location"`
	Locations  []string `json:"locations"`
	Department string   `json:"department"`
	ATSJobID   string   `json:"ats_job_id"`
	TCreate    int64    `json:"t_create"`
	TUpdate    int64    `json:"t_update"`
	URL        string   `json:"canonicalPositionUrl"`
	IsPrivate  bool     `json:"isPrivate"`
}

func (n *Netflix) client() *http.Client {
	if n.Client != nil {
		return n.Client
	}

	return &http.Client{Timeout: 15 * time.Second}
}

// Fetch は全求人を返す。sort_by=old で新規を末尾に寄せ、
// ページング中の位置ずれによる取りこぼしを抑える。
func (n *Netflix) Fetch(ctx context.Context) ([]Job, error) {
	client := n.client()

	var jobs []Job

	seen := make(map[string]struct{})

	for start := 0; start < nflxMaxJobs; start += nflxPageSize {
		if start > 0 {
			if err := sleepCtx(ctx, nflxPageSleep); err != nil {
				return nil, err
			}
		}

		page, count, err := n.fetchPageRetry(ctx, client, start)
		if err != nil {
			return nil, fmt.Errorf("start=%d: %w", start, err)
		}

		for _, j := range page {
			if _, ok := seen[j.ID]; ok {
				continue
			}

			seen[j.ID] = struct{}{}
			jobs = append(jobs, j)
		}

		if len(page) == 0 || start+nflxPageSize >= count {
			break
		}
	}

	return jobs, nil
}

func (n *Netflix) fetchPageRetry(ctx context.Context, client *http.Client, start int) ([]Job, int, error) {
	var page []Job
	var count int

	b := retry.WithMaxRetries(nflxMaxRetries, retry.NewExponential(nflxRetryWait))

	err := retry.Do(ctx, b, func(ctx context.Context) error {
		var err error

		page, count, err = n.fetchPage(ctx, client, start)

		var herr *httpError
		if errors.As(err, &herr) && herr.StatusCode == http.StatusTooManyRequests {
			return retry.RetryableError(err)
		}

		return err
	})

	return page, count, err
}

func (n *Netflix) fetchPage(ctx context.Context, client *http.Client, start int) ([]Job, int, error) {
	q := url.Values{}
	q.Set("domain", "netflix.com")
	q.Set("start", strconv.Itoa(start))
	q.Set("num", strconv.Itoa(nflxPageSize))
	q.Set("sort_by", "old")

	var body nflxResponse
	if err := n.getJSON(ctx, client, nflxBaseURL+"?"+q.Encode(), &body); err != nil {
		return nil, 0, err
	}

	jobs := make([]Job, 0, len(body.Positions))
	for _, raw := range body.Positions {
		var p nflxPosition
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, 0, fmt.Errorf("decode position: %w", err)
		}

		if p.IsPrivate {
			continue
		}

		id := p.ATSJobID
		if id == "" {
			id = strconv.FormatInt(p.ID, 10)
		}

		loc := strings.TrimSpace(p.Location)
		if loc == "" {
			loc = NormalizeMulti(p.Locations)
		}

		jobs = append(jobs, Job{
			ID:         id,
			Title:      p.Name,
			Location:   loc,
			Department: p.Department,
			URL:        p.URL,
			TCreate:    p.TCreate,
			TUpdate:    p.TUpdate,
		})
	}

	return jobs, body.Count, nil
}

func (n *Netflix) getJSON(ctx context.Context, client *http.Client, u string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", nflxUserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close() //nolint

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, nflxErrBodyLimit))

		return &httpError{
			StatusCode: resp.StatusCode,
			msg: fmt.Sprintf("unexpected status: %s (retry-after=%q body=%q)",
				resp.Status, resp.Header.Get("Retry-After"), snippet),
		}
	}

	if err := json.UnmarshalRead(resp.Body, v); err != nil {
		return fmt.Errorf("decode: %w", err)
	}

	return nil
}
