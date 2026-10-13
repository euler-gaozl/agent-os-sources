package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/euler-gaozl/agent-os-sdk/contracts"
	"github.com/euler-gaozl/agent-os-sdk/servicehttp"
	"github.com/jackc/pgx/v5/pgxpool"
)

func managementFixture(t *testing.T) (*Store, *pgxpool.Pool, *atomic.Int32) {
	t.Helper()
	ctx := context.Background()
	admin, role := fixtureDatabase(t)
	if e := Migrate(ctx, admin, role); e != nil {
		t.Fatal(e)
	}
	c := admin.Config().Copy()
	c.ConnConfig.RuntimeParams["role"] = role
	db, e := pgxpool.NewWithConfig(ctx, c)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(db.Close)
	counter := &atomic.Int32{}
	s := &Store{DB: db, AllowedHosts: []string{"www.stats.gov.cn"}, HTTP: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		n := counter.Add(1)
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/html; charset=utf-8"}}, Body: io.NopCloser(strings.NewReader(fmt.Sprintf("<title>夹具</title><p>证据🙂库存%d吨</p>", n)))}, nil
	})}}
	return s, admin, counter
}

func TestCaptureHTTPDiscardedReplyReturnsSameObservation(t *testing.T) {
	s, _, counter := managementFixture(t)
	token := strings.Repeat("c", 40)
	server := httptest.NewServer(s.Handler(servicehttp.Config{Role: "sources", Peers: map[string]string{"control": servicehttp.TokenHash(token)}, Authorize: Authorize}))
	defer server.Close()
	id := contracts.ID()
	input := map[string]string{"capture_id": id, "url": "https://www.stats.gov.cn/fixture"}
	call := func(discard bool) (int, []byte) {
		req, e := http.NewRequest("POST", server.URL+"/captures", bytes.NewReader(contracts.JSON(input)))
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer resp.Body.Close()
		if discard {
			return resp.StatusCode, nil
		}
		body, e := io.ReadAll(resp.Body)
		if e != nil {
			t.Fatal(e)
		}
		return resp.StatusCode, body
	}
	// A real HTTP request commits; deliberately discard its result before retry.
	status, _ := call(true)
	if status != 200 {
		t.Fatal(status)
	}
	stored, e := s.Read(context.Background(), id)
	if e != nil {
		t.Fatal(e)
	}
	status, body := call(false)
	if status != 200 {
		t.Fatal(status, string(body))
	}
	var got Page
	if e = json.Unmarshal(body, &got); e != nil {
		t.Fatal(e)
	}
	if counter.Load() != 1 || got.Content != stored.Content || got.Hash != stored.Hash || !got.FetchedAt.Equal(stored.FetchedAt) || got.ExtractionVersion != extractionVersion {
		t.Fatal("retry created a different observation", counter.Load())
	}
	input["url"] = "https://www.stats.gov.cn/another"
	status, _ = call(false)
	if status != 409 || counter.Load() != 1 {
		t.Fatal("capture identity re-bound")
	}
}

func TestConcurrentCaptureIDsRetainOneCompleteObservation(t *testing.T) {
	s, admin, _ := managementFixture(t)
	ctx := context.Background()
	// Force two HTTP responses to race before either can commit; goroutine
	// scheduling alone would also permit six purely sequential dedup reads.
	entered := make(chan struct{}, 6)
	release := make(chan struct{})
	transport := s.HTTP.Transport
	s.HTTP.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		entered <- struct{}{}
		select {
		case <-release:
			return transport.RoundTrip(r)
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
	})
	id := contracts.ID()
	var wg sync.WaitGroup
	ch := make(chan Capture, 6)
	fail := make(chan error, 6)
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, e := s.FetchWithID(ctx, "https://www.stats.gov.cn/fixture", id)
			if e != nil {
				fail <- e
			} else {
				ch <- c
			}
		}()
	}
	for range 2 {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			close(release)
			wg.Wait()
			t.Fatal("two capture fetches did not reach the controlled race")
		}
	}
	close(release)
	wg.Wait()
	close(ch)
	close(fail)
	for e := range fail {
		t.Fatal(e)
	}
	var first Capture
	for c := range ch {
		if first.ID != "" && (c.Hash != first.Hash || c.Content != first.Content || !c.FetchedAt.Equal(first.FetchedAt)) {
			t.Fatal("concurrent replies disagree")
		}
		first = c
	}
	var count int
	for _, table := range []string{"source_captures", "source_blobs"} {
		if e := admin.QueryRow(ctx, "SELECT count(*) FROM sources."+table).Scan(&count); e != nil || count != 1 {
			t.Fatal("losing capture committed artifacts", table, count, e)
		}
	}
	b, e := s.Bundle(ctx, id)
	if e != nil || b.ID != id || contracts.Hash(b.Raw) != b.RawHash || contracts.Hash([]byte(b.Text)) != b.TextHash || b.Text != first.Content || b.FinalURL != b.RequestedURL {
		t.Fatal("inconsistent bundle", e)
	}
}

func TestCaptureExportBoundsAndIdentityPermissions(t *testing.T) {
	s, admin, _ := managementFixture(t)
	ctx := context.Background()
	c, e := s.Fetch(ctx, "https://www.stats.gov.cn/fixture")
	if e != nil {
		t.Fatal(e)
	}
	peers := map[string]string{}
	for _, role := range []string{"control", "knowledge", "sources", "runtime", "futures", "toolhub"} {
		peers[role] = servicehttp.TokenHash(strings.Repeat(role, 12))
	}
	h := s.Handler(servicehttp.Config{Role: "sources", Peers: peers, Authorize: Authorize})
	for role := range peers {
		for _, path := range []string{"/captures/" + c.ID + "/bundle", "/captures/" + c.ID, "/captures?url=https%3A%2F%2Fwww.stats.gov.cn%2Ffixture", "/capture-capabilities"} {
			r := httptest.NewRequest("GET", path, nil)
			r.Header.Set("Authorization", "Bearer "+strings.Repeat(role, 12))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			allowed := role == "control" || (role == "knowledge" && (strings.HasSuffix(path, "/bundle") || path == "/capture-capabilities"))
			want := 403
			if allowed {
				want = 200
			}
			if w.Code != want {
				t.Fatal(role, path, w.Code, w.Body.String())
			}
		}
	}
	for _, q := range []string{"?offset=-1", "?limit=1201", "?offset=0&offset=1", "?unknown=x"} {
		r := httptest.NewRequest("GET", "/captures/"+c.ID+q, nil)
		r.Header.Set("Authorization", "Bearer "+strings.Repeat("control", 12))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatal(q, w.Code)
		}
	}
	if _, e = admin.Exec(ctx, "UPDATE sources.source_captures SET content=$2 WHERE id=$1", c.ID, strings.Repeat("x", (1<<20)+1)); e != nil {
		t.Fatal(e)
	}
	_, e = s.Bundle(ctx, c.ID)
	var se *servicehttp.StatusError
	if !errors.As(e, &se) || se.Status != 413 {
		t.Fatal("oversize bundle was not refused", e)
	}
}

func TestCaptureHistoryUsesURLIndexAndStablePages(t *testing.T) {
	s, admin, _ := managementFixture(t)
	ctx := context.Background()
	first, e := s.Fetch(ctx, "https://www.stats.gov.cn/fixture")
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 22; i++ {
		_, e = admin.Exec(ctx, `INSERT INTO sources.source_captures(id,url,hash,media_type,title,content,links,fetched_at,extraction_version) SELECT $1,url,hash,media_type,title,content,links,fetched_at,extraction_version FROM sources.source_captures WHERE id=$2`, contracts.ID(), first.ID)
		if e != nil {
			t.Fatal(e)
		}
	}
	p, e := s.History(ctx, first.URL, "")
	if e != nil || len(p.Items) != 20 || p.Next == "" {
		t.Fatal(p, e)
	}
	q, e := s.History(ctx, first.URL, p.Next)
	if e != nil || len(q.Items) != 3 || q.Next != "" {
		t.Fatal(q, e)
	}
	seen := map[string]bool{}
	for _, item := range append(p.Items, q.Items...) {
		if seen[item.ID] {
			t.Fatal("history duplicated a capture")
		}
		seen[item.ID] = true
	}
	if _, e = s.History(ctx, "https://www.stats.gov.cn/other", p.Next); e == nil {
		t.Fatal("cross-source cursor accepted")
	}
}

func TestLegacyCaptureMigrationPreservesUnknownExtractionVersion(t *testing.T) {
	admin, role := fixtureDatabase(t)
	ctx := context.Background()
	legacy := strings.Split(migrationSQL, "ALTER TABLE")[0]
	if _, e := admin.Exec(ctx, "CREATE SCHEMA sources; SET search_path=sources; "+legacy); e != nil {
		t.Fatal(e)
	}
	id := contracts.ID()
	raw := []byte("legacy raw")
	hash := contracts.Hash(raw)
	if _, e := admin.Exec(ctx, "INSERT INTO source_blobs(hash,body) VALUES($1,$2)", hash, raw); e != nil {
		t.Fatal(e)
	}
	if _, e := admin.Exec(ctx, `INSERT INTO source_captures(id,url,hash,media_type,title,content,links) VALUES($1,'https://www.stats.gov.cn/old',$2,'text/plain','legacy','旧资料','[]')`, id, hash); e != nil {
		t.Fatal(e)
	}
	for range 2 {
		if e := Migrate(ctx, admin, role); e != nil {
			t.Fatal(e)
		}
	}
	s := Store{DB: admin}
	c, e := s.Read(ctx, id)
	if e != nil || c.Content != "旧资料" || c.Hash != hash || c.ExtractionVersion != "unrecorded" {
		t.Fatal("legacy provenance invented or content altered", e)
	}
}
