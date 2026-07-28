package store

import (
	"database/sql"
	"jobwatch/internal/fetcher"
)

// JobRow は jobs テーブル1行の Go 表現。
// SELECT の scan 先と、fetch 結果から作る新しい状態の両方に使う。
type JobRow struct {
	JobID       string
	Title       string
	Location    string
	Department  string
	URL         string
	TCreate     int64
	TUpdate     int64
	ContentHash string
	ClosedAt    sql.NullInt64 // fetch 由来では常に NULL(掲載中)
}

func NewJobRow(j fetcher.Job) JobRow {
	return JobRow{
		JobID:       j.ID,
		Title:       j.Title,
		Location:    j.Location,
		Department:  j.Department,
		URL:         j.URL,
		TCreate:     j.TCreate,
		TUpdate:     j.TUpdate,
		ContentHash: j.ContentHash(),
	}
}
