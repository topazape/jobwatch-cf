-- 求人の現在状態(1求人 = 1行)
CREATE TABLE jobs (
  source        TEXT NOT NULL,     -- fetcher の slug
  job_id        TEXT NOT NULL,     -- ソース側の求人ID(Netflix なら ats_job_id)
  title         TEXT NOT NULL,
  location      TEXT,
  department    TEXT,
  url           TEXT,
  t_create      INTEGER,           -- unix秒(ソース由来。0 = 非提供)
  t_update      INTEGER,           -- unix秒(ソース由来。信頼性はソース依存)
  content_hash  TEXT NOT NULL,     -- Go の ContentHash と文字列比較して変更検知
  first_seen_at INTEGER NOT NULL,  -- unix秒(Worker の時計)
  closed_at     INTEGER,           -- NULL = 掲載中
  PRIMARY KEY (source, job_id)
) STRICT;

-- 変化の履歴(追記のみ)。1求人 × 1実行 = 最大1行
CREATE TABLE job_events (
  id     INTEGER PRIMARY KEY,
  source TEXT NOT NULL,
  job_id TEXT NOT NULL,
  event  TEXT NOT NULL CHECK (event IN ('added', 'changed', 'closed', 'reopened')),
  at     INTEGER NOT NULL,         -- unix秒(Worker の時計)
  detail TEXT                      -- changed/reopened の変更内容 JSON: {"field":{"old":…,"new":…}}。変更なしは NULL
) STRICT;

-- 同期実行の記録(1回の cron × 1ソース = 1行)
CREATE TABLE runs (
  id         INTEGER PRIMARY KEY,
  source     TEXT NOT NULL,
  ran_at     INTEGER NOT NULL,     -- unix秒
  error      TEXT NOT NULL DEFAULT '',  -- '' = 成功
  jobs_count INTEGER               -- スナップショット件数。失敗時 NULL
) STRICT;
