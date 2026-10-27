package management

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/lucasdevco/tokenhub/internal/database/queries"
)

type pageCursor struct {
	Started int64  `json:"started"`
	Key     string `json:"key"`
}

func cursorText(c pageCursor) string {
	data, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(data)
}
func parseCursor(r *http.Request) (pageCursor, error) {
	c := pageCursor{Started: math.MaxInt64, Key: "\uffff"}
	if value := r.URL.Query().Get("cursor"); value != "" {
		if len(value) > 4096 {
			return c, errors.New("invalid cursor")
		}
		data, err := base64.RawURLEncoding.DecodeString(value)
		if err != nil {
			return c, errors.New("invalid cursor")
		}
		if json.Unmarshal(data, &c) != nil || c.Key == "" {
			return c, errors.New("invalid cursor")
		}
	}
	return c, nil
}
func indexQueryError(w http.ResponseWriter, err error) {
	if errors.Is(err, context.DeadlineExceeded) {
		http.Error(w, "查询超时，请缩小搜索范围", http.StatusServiceUnavailable)
		return
	}
	http.Error(w, "日志索引查询失败: "+err.Error(), http.StatusInternalServerError)
}
func (s *Store) status() (uint64, string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.revision, s.problem, s.indexing
}

func (s *Store) list(w http.ResponseWriter, r *http.Request) {
	limit, offset, err := page(r)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	cursor, err := parseCursor(r)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if r.URL.Query().Get("cursor") != "" {
		offset = 0
	}
	now := time.Now()
	histogram, err := newRequestHistogram(r.URL.Query().Get("hours"), now)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	state, method, search := r.URL.Query().Get("state"), r.URL.Query().Get("method"), strings.ToLower(r.URL.Query().Get("q"))
	totals, err := s.db.Q.SummaryTotals(ctx, s.dir)
	if err != nil {
		indexQueryError(w, err)
		return
	}
	var total, all, unfinished, failed int64
	for _, group := range totals {
		all += group.N
		if group.State == "unfinished" {
			unfinished += group.N
		}
		if group.Failed != 0 {
			failed += group.N
		}
		if state != "" && state != group.State && !(state == "capture_error" && group.CaptureError != 0) {
			continue
		}
		if method != "" && method != group.Method {
			continue
		}
		total += group.N
	}
	if search != "" {
		total, err = s.db.Q.CountSearch(ctx, queries.CountSearchParams{Source: s.dir, Search: search, State: state, Method: method})
		if err != nil {
			indexQueryError(w, err)
			return
		}
	}
	first := histogram.Buckets[0].Started.UnixNano()
	var bins []queries.HistogramRow
	if search == "" {
		bins, err = s.db.Q.Histogram(ctx, queries.HistogramParams{Source: s.dir, FirstBucket: first / 120000000000, LastBucket: now.UnixNano() / 120000000000, State: state, Method: method})
	} else {
		var rows []queries.SearchHistogramRow
		rows, err = s.db.Q.SearchHistogram(ctx, queries.SearchHistogramParams{Source: s.dir, StartTime: first, EndTime: now.UnixNano(), Search: search, State: state, Method: method})
		for _, row := range rows {
			bins = append(bins, queries.HistogramRow{Bucket: row.Bucket, N: row.N})
		}
	}
	if err != nil {
		indexQueryError(w, err)
		return
	}
	for _, bin := range bins {
		index := (bin.Bucket*120000000000 - first) / (int64(histogram.IntervalSeconds) * int64(time.Second))
		if index >= 0 && index < int64(len(histogram.Buckets)) {
			histogram.Buckets[index].Count += int(bin.N)
			histogram.Total += int(bin.N)
		}
	}
	rows, err := s.db.Q.ListRecords(ctx, queries.ListRecordsParams{Source: s.dir, BeforeStarted: cursor.Started, BeforeKey: cursor.Key, State: state, Method: method, Search: search, PageSize: int64(limit + 1), PageOffset: int64(offset)})
	if err != nil {
		indexQueryError(w, err)
		return
	}
	next := ""
	if len(rows) > limit {
		last := rows[limit-1]
		next = cursorText(pageCursor{Started: last.Started, Key: last.Key})
		rows = rows[:limit]
	}
	items := make([]Transaction, 0, len(rows))
	for _, row := range rows {
		var item Transaction
		if err := json.Unmarshal(row.Summary, &item); err != nil {
			indexQueryError(w, err)
			return
		}
		items = append(items, item)
	}
	revision, problem, indexing := s.status()
	send(w, map[string]any{"items": items, "total": total, "all": all, "unfinished": unfinished, "errors": failed, "revision": revision, "problem": problem, "indexing": indexing, "next_cursor": next, "histogram": histogram})
}

func (s *Store) listFiles(w http.ResponseWriter, r *http.Request) {
	limit, offset, err := page(r)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	cursor, err := parseCursor(r)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if r.URL.Query().Get("cursor") != "" {
		offset = 0
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	search := strings.ToLower(r.URL.Query().Get("q"))
	var total int64
	if search == "" {
		total, err = s.db.Q.GetSetting(ctx, "file_count:"+s.dir)
		if errors.Is(err, sql.ErrNoRows) {
			err = nil
		}
	} else {
		total, err = s.db.Q.CountFileSearch(ctx, queries.CountFileSearchParams{Search: search, Source: s.dir})
	}
	if err != nil {
		indexQueryError(w, err)
		return
	}
	rows, err := s.db.Q.ListFiles(ctx, queries.ListFilesParams{Source: s.dir, BeforeName: cursor.Key, Search: search, PageSize: int64(limit + 1), PageOffset: int64(offset)})
	if err != nil {
		indexQueryError(w, err)
		return
	}
	next := ""
	if len(rows) > limit {
		next = cursorText(pageCursor{Key: rows[limit-1].Name})
		rows = rows[:limit]
	}
	files := make([]File, 0, len(rows))
	for _, row := range rows {
		files = append(files, File{Name: row.Name, Size: row.Size, Modified: time.Unix(0, row.Modified), Problem: row.Problem})
	}
	_, problem, indexing := s.status()
	send(w, map[string]any{"items": files, "total": total, "problem": problem, "indexing": indexing, "next_cursor": next})
}
