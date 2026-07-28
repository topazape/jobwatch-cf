package store

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"
	"fmt"
)

// Snapshot は1回の実行で観測した1ソース分の結果。Sync への入力。
type Snapshot struct {
	Source string   // fetcher の slug。jobs.source 列に入る
	Error  string   // fetch 失敗時のメッセージ。"" = 成功
	Jobs   []JobRow // 取得できた求人(失敗時は nil)
}

// Stats は1回の Sync で起きた変化の件数。
type Stats struct {
	Added    int
	Changed  int
	Closed   int
	Reopened int
}

// Sync は Snapshot を D1 へ反映し、runs に実行記録を残す。
// fetch 失敗・0件のときは jobs に触らず、runs へのエラー記録のみ行う。
func Sync(ctx context.Context, db *sql.DB, now int64, snap Snapshot) (Stats, error) {
	errMsg := snap.Error
	if errMsg == "" && len(snap.Jobs) == 0 {
		// fetch 成功で 0 件はソース側の異常の可能性が高い。
		// そのまま差分を取ると全求人を closed 化してしまうので弾く
		errMsg = "empty snapshot"
	}

	if errMsg != "" {
		if err := recordRun(ctx, db, snap.Source, now, errMsg, sql.NullInt64{}); err != nil {
			return Stats{}, fmt.Errorf("record run: %w", err)
		}

		return Stats{}, errors.New(errMsg)
	}

	stats, err := applyDiff(ctx, db, snap.Source, now, snap.Jobs)
	if err != nil {
		// 途中失敗も runs に残す(ベストエフォート)。記録自体の失敗は元のエラーに併記
		if rerr := recordRun(ctx, db, snap.Source, now, err.Error(), sql.NullInt64{}); rerr != nil {
			err = errors.Join(err, rerr)
		}

		return stats, err
	}

	jobsCount := sql.NullInt64{Int64: int64(len(snap.Jobs)), Valid: true}
	if err := recordRun(ctx, db, snap.Source, now, "", jobsCount); err != nil {
		return stats, fmt.Errorf("record run: %w", err)
	}

	return stats, nil
}

// applyDiff はスナップショットと DB の既存行の差分を検出して書き込む。
// d1 ドライバはトランザクション非対応のため1文ずつ書く。jobs(状態)を先、
// job_events(履歴)を後にすることで、途中で落ちても次回実行が hash 一致で収束する。
func applyDiff(ctx context.Context, db *sql.DB, source string, now int64, fetched []JobRow) (Stats, error) {
	var st Stats

	existing, err := loadJobs(ctx, db, source)
	if err != nil {
		return st, fmt.Errorf("load jobs: %w", err)
	}

	for _, j := range fetched {
		old, ok := existing[j.JobID]
		delete(existing, j.JobID) // ループ後に残った行 = 今回観測されなかった行 = closed 候補

		switch {
		case !ok: // DB に無い → added
			if err := insertJob(ctx, db, source, now, j); err != nil {
				return st, fmt.Errorf("insert %s: %w", j.JobID, err)
			}

			if err := insertEvent(ctx, db, source, j.JobID, "added", now, nil); err != nil {
				return st, fmt.Errorf("event added %s: %w", j.JobID, err)
			}

			st.Added++
		case old.ClosedAt.Valid: // closed 済みが再出現 → reopened
			if err := updateJob(ctx, db, source, j); err != nil {
				return st, fmt.Errorf("update %s: %w", j.JobID, err)
			}

			if err := insertEvent(ctx, db, source, j.JobID, "reopened", now, diffDetail(old, j)); err != nil {
				return st, fmt.Errorf("event reopened %s: %w", j.JobID, err)
			}

			st.Reopened++
		case old.ContentHash != j.ContentHash: // 内容が変わった → changed
			if err := updateJob(ctx, db, source, j); err != nil {
				return st, fmt.Errorf("update %s: %w", j.JobID, err)
			}

			if err := insertEvent(ctx, db, source, j.JobID, "changed", now, diffDetail(old, j)); err != nil {
				return st, fmt.Errorf("event changed %s: %w", j.JobID, err)
			}

			st.Changed++
		}
		// どの case にも該当しない = 掲載中かつ変更なし。行にもイベントにも触らない
	}

	// スナップショットに無かった掲載中の行を closed 化する
	for id, old := range existing {
		if old.ClosedAt.Valid {
			continue // 過去の実行で closed 済み
		}

		if err := closeJob(ctx, db, source, id, now); err != nil {
			return st, fmt.Errorf("close %s: %w", id, err)
		}

		if err := insertEvent(ctx, db, source, id, "closed", now, nil); err != nil {
			return st, fmt.Errorf("event closed %s: %w", id, err)
		}

		st.Closed++
	}

	return st, nil
}

type fieldChange struct {
	Old any `json:"old"`
	New any `json:"new"`
}

// diffDetail は job_events.detail 用の JSON(string)を返す。差分がなければ nil(= NULL)。
// STRICT テーブルの TEXT 列に []byte を渡すと BLOB 扱いで弾かれるため string にする。
func diffDetail(old, cur JobRow) any {
	d := map[string]fieldChange{}

	if old.Title != cur.Title {
		d["title"] = fieldChange{Old: old.Title, New: cur.Title}
	}

	if old.Location != cur.Location {
		d["location"] = fieldChange{Old: old.Location, New: cur.Location}
	}

	if old.Department != cur.Department {
		d["department"] = fieldChange{Old: old.Department, New: cur.Department}
	}

	if old.URL != cur.URL {
		d["url"] = fieldChange{Old: old.URL, New: cur.URL}
	}

	if old.TCreate != cur.TCreate {
		d["t_create"] = fieldChange{Old: old.TCreate, New: cur.TCreate}
	}

	if old.TUpdate != cur.TUpdate {
		d["t_update"] = fieldChange{Old: old.TUpdate, New: cur.TUpdate}
	}

	if len(d) == 0 {
		return nil
	}

	b, err := json.Marshal(d)
	if err != nil {
		return nil // string/int64 のみなので到達しない
	}

	return string(b)
}
