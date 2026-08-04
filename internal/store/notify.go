package store

import (
	"context"
	"database/sql"
	"fmt"
)

type PendingEvent struct {
	ID       int64
	Source   string
	Event    string
	Title    string
	Location string
	URL      string
}

func PendingEvents(ctx context.Context, db *sql.DB) ([]PendingEvent, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT e.id, e.source, e.event, j.title, j.location, j.url
		FROM job_events AS e
		INNER JOIN jobs AS j
		ON e.job_id = j.job_id AND e.source = j.source
		WHERE e.notified_at IS NULL
		  AND e.event IN ('added', 'changed', 'closed', 'reopened')
		  AND j.location LIKE '%Japan%'
		ORDER BY e.id`)
	if err != nil {
		return nil, err
	}

	defer rows.Close() //nolint:errcheck

	var events []PendingEvent

	for rows.Next() {
		var e PendingEvent
		if err := rows.Scan(
			&e.ID,
			&e.Source,
			&e.Event,
			&e.Title,
			&e.Location,
			&e.URL); err != nil {
			return nil, err
		}

		events = append(events, e)
	}

	return events, rows.Err()
}

func MarkNotified(ctx context.Context, db *sql.DB, ids []int64, now int64) error {
	for _, id := range ids {
		if _, err := db.ExecContext(ctx,
			`UPDATE job_events SET notified_at = ? WHERE id = ?`,
			now, id); err != nil {
			return fmt.Errorf("mark notified %d: %w", id, err)
		}
	}

	return nil
}
