package store

import (
	"context"
	"database/sql"
)

// loadJobs は source の既存行を job_id で引ける map にして返す。
func loadJobs(ctx context.Context, db *sql.DB, source string) (map[string]JobRow, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT job_id, title, location, department, url,
		       t_create, t_update, content_hash, closed_at
		FROM jobs
		WHERE source = ?`, source)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	m := make(map[string]JobRow)

	for rows.Next() {
		var j JobRow
		if err := rows.Scan(
			&j.JobID,
			&j.Title,
			&j.Location,
			&j.Department,
			&j.URL,
			&j.TCreate,
			&j.TUpdate,
			&j.ContentHash,
			&j.ClosedAt,
		); err != nil {
			return nil, err
		}

		m[j.JobID] = j
	}

	// database/sql を使う時の定番イディオム
	// nil を返すと途中でエラーが発生したことを検知できない
	return m, rows.Err()
}

// insertJob は新規求人を追加する。closed_at は NULL(掲載中)で入る。
func insertJob(ctx context.Context, db *sql.DB, source string, now int64, j JobRow) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO jobs (source, job_id, title, location, department, url,
		                  t_create, t_update, content_hash, first_seen_at, closed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)`,
		source, j.JobID, j.Title, j.Location, j.Department, j.URL,
		j.TCreate, j.TUpdate, j.ContentHash, now)

	return err
}

// updateJob は changed / reopened 共用。掲載が確認できた行なので closed_at も NULL に戻す。
// first_seen_at は初回掲載の記録なので触らない。
func updateJob(ctx context.Context, db *sql.DB, source string, j JobRow) error {
	_, err := db.ExecContext(ctx, `
		UPDATE jobs
		SET title = ?, location = ?, department = ?, url = ?,
		    t_create = ?, t_update = ?, content_hash = ?, closed_at = NULL
		WHERE source = ? AND job_id = ?`,
		j.Title, j.Location, j.Department, j.URL,
		j.TCreate, j.TUpdate, j.ContentHash, source, j.JobID)

	return err
}

// closeJob は掲載終了を記録する。
func closeJob(ctx context.Context, db *sql.DB, source, jobID string, now int64) error {
	_, err := db.ExecContext(ctx,
		`UPDATE jobs SET closed_at = ? WHERE source = ? AND job_id = ?`,
		now, source, jobID)

	return err
}

// insertEvent は job_events に1行追記する。detail は nil(= NULL)か JSON 文字列。
func insertEvent(ctx context.Context, db *sql.DB, source, jobID, event string, at int64, detail any) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO job_events (source, job_id, event, at, detail)
		VALUES (?, ?, ?, ?, ?)`,
		source, jobID, event, at, detail)

	return err
}

// recordRun は runs に1行追記する。
func recordRun(ctx context.Context, db *sql.DB, source string, at int64, errMsg string, jobsCount sql.NullInt64) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO runs (source, ran_at, error, jobs_count) VALUES (?, ?, ?, ?)`,
		source, at, errMsg, jobsCount)

	return err
}
