package notify

import (
	"context"
	"database/sql"
	"fmt"
	"jobwatch/internal/store"
	"strings"
)

// sendLimit は1メッセージに載せる求人数の上限。ntfy は本文が大きすぎると
// 添付ファイル化されるため、超過分は件数のみ伝える
const sendLimit = 20

// SendPending は未通知イベントを1メッセージで送り、成功したら通知済みにする。
// 送った件数を返す。対象0件なら何も送らない。
func SendPending(ctx context.Context, db *sql.DB, n *Ntfy, now int64) (int, error) {
	events, err := store.PendingEvents(ctx, db)
	if err != nil {
		return 0, fmt.Errorf("pending events: %w", err)
	}

	if len(events) == 0 {
		return 0, nil
	}

	if err := n.Notify(ctx, format(events)); err != nil {
		return 0, fmt.Errorf("send: %w", err)
	}

	ids := make([]int64, 0, len(events))
	for _, e := range events {
		ids = append(ids, e.ID)
	}

	if err := store.MarkNotified(ctx, db, ids, now); err != nil {
		return len(events), fmt.Errorf("mark: %w", err)
	}

	return len(events), nil
}

// format は通知本文を組み立てる。
func format(events []store.PendingEvent) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Job updates: %d\n", len(events))

	for i, e := range events {
		if i == sendLimit {
			fmt.Fprintf(&b, "...and %d more", len(events)-sendLimit)

			break
		}

		fmt.Fprintf(&b, "* [%s/%s] %s - %s\n  %s\n", e.Source, e.Event, e.Title, e.Location, e.URL)
	}

	return strings.TrimRight(b.String(), "\n")
}
