package fetcher

import (
	"slices"
	"strings"
	"time"
)

// multiSep は複数値フィールドの区切り。値自体に含まれないことを前提とする。
const multiSep = "; "

// NormalizeMulti は複数値を元順序のまま単一文字列へ畳み込む。
func NormalizeMulti(values []string) string {
	var out []string
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}

	return strings.Join(out, multiSep)
}

// sortedMulti は畳み込み済み文字列を順序非依存にする(ContentHash 用)。
func sortedMulti(s string) string {
	parts := strings.Split(s, multiSep)
	slices.Sort(parts)

	return strings.Join(parts, multiSep)
}

// ParseDateUTC はタイムゾーンなしの日付文字列を UTC 0 時の unix 秒にする。
// パース不能なら 0(= ソース非提供)。
func ParseDateUTC(layout, s string) int64 {
	t, err := time.ParseInLocation(layout, strings.TrimSpace(s), time.UTC)
	if err != nil {
		return 0
	}

	return t.Unix()
}
