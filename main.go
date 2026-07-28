package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"jobwatch/internal/fetcher"
	"jobwatch/internal/store"
	"log"
	"time"

	"github.com/syumai/workers/cloudflare/cron"
	"github.com/syumai/workers/cloudflare/d1"
	"github.com/syumai/workers/cloudflare/fetch"
	"golang.org/x/sync/errgroup"
)

const fetchTimeout = 10 * time.Minute

func task(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(
		ctx, fetchTimeout,
	)
	defer cancel()

	// D1 は fetch より先に開く。binding の設定ミスを fetch を待たずに検出する
	connector, err := d1.OpenConnector("DB")
	if err != nil {
		return err
	}

	db := sql.OpenDB(connector)
	defer func() {
		_ = db.Close()
	}()

	// 実行全体の統一タイムスタンプ。first_seen_at / at / ran_at を揃え、
	// job_events と runs を (source, at = ran_at) で突き合わせられるようにする
	now := time.Now().Unix()

	fetchers := []fetcher.Fetcher{
		&fetcher.Netflix{
			Client: fetch.NewClient().HTTPClient(fetch.RedirectModeFollow),
		},
	}

	results := make([]store.Snapshot, len(fetchers))

	g, gctx := errgroup.WithContext(ctx)
	for i, f := range fetchers {
		g.Go(func() error {
			jobs, err := f.Fetch(gctx)
			if err != nil {
				log.Printf("%s: fetch failed: %v", f.Slug(), err)
				results[i] = store.Snapshot{Source: f.Slug(), Error: err.Error()}

				return nil
			}

			out := make([]store.JobRow, 0, len(jobs))
			for _, j := range jobs {
				out = append(out, store.NewJobRow(j))
			}

			log.Printf("%s: %d jobs", f.Slug(), len(out))
			results[i] = store.Snapshot{Source: f.Slug(), Jobs: out}

			return nil
		})
	}

	_ = g.Wait()

	// sync はソースごとに逐次。異常は runs に記録された上でここにも返り、
	// cron 実行自体を失敗にしてログで気づけるようにする
	var errs []error

	for _, snap := range results {
		st, err := store.Sync(ctx, db, now, snap)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", snap.Source, err))

			continue
		}

		log.Printf("%s: synced: added=%d changed=%d closed=%d reopened=%d",
			snap.Source, st.Added, st.Changed, st.Closed, st.Reopened)
	}

	return errors.Join(errs...)
}

func main() {
	cron.ScheduleTask(task)
}
