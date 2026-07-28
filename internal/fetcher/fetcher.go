package fetcher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

type Job struct {
	ID         string
	Title      string
	Location   string
	Department string
	URL        string
	TCreate    int64
	TUpdate    int64
}

// ContentHash は変更検知用ハッシュ。固定フィールド順で決定的。
// 複数値フィールドはソートしてから畳み込み、配列順序の揺れを偽検知にしない。
// TUpdate は信頼性がソース依存のため含めない。
func (j Job) ContentHash() string {
	h := sha256.New()
	_, _ = fmt.Fprintf(
		h,
		"%s\x1f%s\x1f%s\x1f%s\x1f%s\x1f%d",
		j.ID, j.Title, sortedMulti(j.Location), sortedMulti(j.Department), j.URL, j.TCreate,
	)

	return hex.EncodeToString(h.Sum(nil))
}

type Fetcher interface {
	Slug() string // ticker symbol
	Fetch(ctx context.Context) ([]Job, error)
}

// sleepCtx は ctx キャンセルに反応する sleep。
func sleepCtx(ctx context.Context, d time.Duration) error {
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
