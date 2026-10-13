package sources

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/euler-gaozl/agent-os-sdk/contracts"
	"github.com/euler-gaozl/agent-os-sdk/servicehttp"
	"github.com/jackc/pgx/v5"
)

var captureID = regexp.MustCompile(`^[a-f0-9]{32}$`)
var captureBundlePath = regexp.MustCompile(`^/captures/[a-f0-9]{32}/bundle$`)

// A fixed, authenticated service export, never a caller-directed download URL.
// Raw means the exact response body retained by the HTTP client, after any
// transport/content decoding performed by net/http, not original network bytes.
type CaptureBundle struct {
	Format            int       `json:"format"`
	ID                string    `json:"capture_id"`
	RequestedURL      string    `json:"requested_url"`
	FinalURL          string    `json:"final_url"`
	MediaType         string    `json:"media_type"`
	FetchedAt         time.Time `json:"fetched_at"`
	Title             string    `json:"title"`
	ExtractionVersion string    `json:"extraction_version"`
	RawHash           string    `json:"raw_sha256"`
	TextHash          string    `json:"text_sha256"`
	RawBytes          int       `json:"raw_bytes"`
	TextBytes         int       `json:"text_bytes"`
	Raw               []byte    `json:"raw"`
	Text              string    `json:"text"`
}

func captureError(status int, message string) error {
	return &servicehttp.StatusError{Status: status, Message: message}
}

func (s *Store) Bundle(ctx context.Context, id string) (CaptureBundle, error) {
	var b CaptureBundle
	if !captureID.MatchString(id) {
		return b, captureError(400, "invalid capture ID")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var text []byte
	// The single statement gives one immutable observation. Limit fields before
	// transferring bytes from PostgreSQL; larger captures stay intact in Sources.
	err := s.DB.QueryRow(ctx, `SELECT c.id,c.url,c.media_type,c.fetched_at,c.title,c.extraction_version,c.hash,
octet_length(b.body),octet_length(c.content),substring(b.body FROM 1 FOR 1048577),substring(convert_to(c.content,'UTF8') FROM 1 FOR 1048577)
FROM source_captures c JOIN source_blobs b ON b.hash=c.hash WHERE c.id=$1`, id).Scan(&b.ID, &b.RequestedURL, &b.MediaType, &b.FetchedAt, &b.Title, &b.ExtractionVersion, &b.RawHash, &b.RawBytes, &b.TextBytes, &b.Raw, &text)
	if errors.Is(err, pgx.ErrNoRows) {
		return b, captureError(404, "capture not found")
	}
	if err != nil {
		return b, err
	}
	if b.RawBytes < 1 || b.RawBytes > 1<<20 || b.TextBytes < 1 || b.TextBytes > 1<<20 {
		return b, captureError(413, "capture exceeds the Knowledge import profile (1 MiB raw and text); retained source is unchanged")
	}
	b.Text = string(text)
	if contracts.Hash(b.Raw) != b.RawHash {
		return b, captureError(409, "capture raw integrity mismatch")
	}
	b.Format = 1
	b.FinalURL = b.RequestedURL // Fetch rejects all HTTP redirects.
	b.TextHash = contracts.Hash([]byte(b.Text))
	if len(contracts.JSON(b)) > 4<<20 {
		return b, captureError(413, "encoded capture bundle exceeds 4 MiB")
	}
	return b, nil
}

type CaptureSummary struct {
	ID                string    `json:"capture_id"`
	URL               string    `json:"url"`
	Title             string    `json:"title"`
	FetchedAt         time.Time `json:"fetched_at"`
	RawHash           string    `json:"raw_sha256"`
	TextBytes         int       `json:"text_bytes"`
	ExtractionVersion string    `json:"extraction_version"`
}
type CaptureHistory struct {
	Items []CaptureSummary `json:"items"`
	Next  string           `json:"next_cursor"`
}

func (s *Store) History(ctx context.Context, sourceURL, after string) (CaptureHistory, error) {
	result := CaptureHistory{Items: []CaptureSummary{}}
	u, e := url.Parse(sourceURL)
	if e != nil || len(sourceURL) > 2000 || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || (after != "" && !captureID.MatchString(after)) {
		return result, captureError(400, "exact HTTPS source URL and valid cursor required")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var before time.Time
	if after != "" {
		e = s.DB.QueryRow(ctx, "SELECT fetched_at FROM source_captures WHERE id=$1 AND url=$2", after, sourceURL).Scan(&before)
		if errors.Is(e, pgx.ErrNoRows) {
			return result, captureError(400, "cursor does not belong to this source URL")
		}
		if e != nil {
			return result, e
		}
	}
	rows, e := s.DB.Query(ctx, `SELECT id,url,substring(title FROM 1 FOR 256),fetched_at,hash,octet_length(content),extraction_version
FROM source_captures WHERE url=$1 AND ($2='' OR (fetched_at,id)<($3,$2)) ORDER BY fetched_at DESC,id DESC LIMIT 21`, sourceURL, after, before)
	if e != nil {
		return result, e
	}
	defer rows.Close()
	for rows.Next() {
		var item CaptureSummary
		if e = rows.Scan(&item.ID, &item.URL, &item.Title, &item.FetchedAt, &item.RawHash, &item.TextBytes, &item.ExtractionVersion); e != nil {
			return result, e
		}
		result.Items = append(result.Items, item)
	}
	if e = rows.Err(); e != nil {
		return result, e
	}
	if len(result.Items) > 20 {
		result.Items = result.Items[:20]
		result.Next = result.Items[19].ID
	}
	return result, nil
}

func parseCaptureQuery(r *http.Request, allowed ...string) (url.Values, error) {
	q, e := url.ParseQuery(r.URL.RawQuery)
	if e != nil {
		return nil, captureError(400, "invalid capture query")
	}
	for k, v := range q {
		found := false
		for _, name := range allowed {
			found = found || k == name
		}
		if !found || len(v) != 1 || v[0] == "" {
			return nil, captureError(400, "invalid capture query")
		}
	}
	return q, nil
}

func (s *Store) managementRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /capture-capabilities", func(w http.ResponseWriter, r *http.Request) {
		servicehttp.JSONResponse(w, 200, map[string]any{"protocol": "source-capture-v1", "durable_capture_ids": true, "bundle_raw_limit": 1 << 20, "bundle_text_limit": 1 << 20, "redirects": false})
	})
	m.HandleFunc("POST /captures", servicehttp.Wrap(func(w http.ResponseWriter, r *http.Request) error {
		var in struct {
			ID  string `json:"capture_id"`
			URL string `json:"url"`
		}
		if e := servicehttp.Decode(w, r, &in); e != nil {
			return e
		}
		c, e := s.FetchWithID(r.Context(), in.URL, in.ID)
		if e != nil {
			return e
		}
		p, e := page(c, 0, 1200)
		if e != nil {
			return e
		}
		w.Header().Set("Cache-Control", "no-store")
		servicehttp.JSONResponse(w, 200, p)
		return nil
	}))
	m.HandleFunc("GET /captures", servicehttp.Wrap(func(w http.ResponseWriter, r *http.Request) error {
		q, e := parseCaptureQuery(r, "url", "after")
		if e != nil {
			return e
		}
		out, e := s.History(r.Context(), q.Get("url"), q.Get("after"))
		if e != nil {
			return e
		}
		w.Header().Set("Cache-Control", "no-store")
		servicehttp.JSONResponse(w, 200, out)
		return nil
	}))
	m.HandleFunc("GET /captures/{id}", servicehttp.Wrap(func(w http.ResponseWriter, r *http.Request) error {
		id := r.PathValue("id")
		if !captureID.MatchString(id) {
			return captureError(400, "invalid capture ID")
		}
		q, e := parseCaptureQuery(r, "offset", "limit")
		if e != nil {
			return e
		}
		offset, limit := 0, 1200
		for _, field := range []string{"offset", "limit"} {
			if q.Get(field) != "" {
				n, err := strconv.Atoi(q.Get(field))
				if err != nil {
					return captureError(400, "invalid capture range")
				}
				if field == "offset" {
					offset = n
				} else {
					limit = n
				}
			}
		}
		c, e := s.Read(r.Context(), id)
		if errors.Is(e, pgx.ErrNoRows) {
			return captureError(404, "capture not found")
		}
		if e != nil {
			return e
		}
		p, e := page(c, offset, limit)
		if e != nil {
			return captureError(400, "invalid capture range")
		}
		w.Header().Set("Cache-Control", "no-store")
		servicehttp.JSONResponse(w, 200, p)
		return nil
	}))
	m.HandleFunc("GET /captures/{id}/bundle", servicehttp.Wrap(func(w http.ResponseWriter, r *http.Request) error {
		if strings.TrimSpace(r.URL.RawQuery) != "" {
			return captureError(400, "capture bundle does not accept queries")
		}
		b, e := s.Bundle(r.Context(), r.PathValue("id"))
		if e != nil {
			return e
		}
		w.Header().Set("Cache-Control", "no-store")
		servicehttp.JSONResponse(w, 200, b)
		return nil
	}))
}
