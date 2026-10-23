package cataloghttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/performance/adapters/perfeng"
)

func fixtures(t *testing.T) []json.RawMessage {
	t.Helper()
	data, err := os.ReadFile("../perfeng/snapshot/pages.json")
	if err != nil {
		t.Fatal(err)
	}
	var pages []json.RawMessage
	if err := json.Unmarshal(data, &pages); err != nil {
		t.Fatal(err)
	}

	return pages
}

func source(endpoint string) Source {
	return Source{Endpoint: endpoint, Token: "test-token", Clock: func() time.Time {
		return time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	}, Expected: perfeng.Expectations{
		Issuer: "https://provider.example/identity", PrincipalIssuer: "https://auth.example/tenant",
		PrincipalID: "argus-service", ProducerID: "perfeng-control-plane", ProducerVersion: "1.0.0",
	}}
}

type catalogHandler struct {
	testing       *testing.T
	pages         []json.RawMessage
	cursor        string
	restart       bool
	calls, starts atomic.Int32
	expired       atomic.Bool
}

func (handler *catalogHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	handler.calls.Add(1)
	if r.Method != http.MethodGet || r.URL.Path != "/v1/performance-catalog" ||
		r.Header.Get("Authorization") != "Bearer test-token" || r.Header.Get("Accept") != "application/json" {
		handler.testing.Error("wrong authenticated endpoint request")
	}
	index := 0
	if r.URL.RawQuery == "" {
		handler.starts.Add(1)
	} else {
		index = 1
		if len(r.URL.Query()) != 1 || r.URL.Query().Get("cursor") != handler.cursor {
			handler.testing.Error("cursor escaped configured query")
		}
		if handler.restart && handler.expired.CompareAndSwap(false, true) {
			w.WriteHeader(http.StatusGone)
			return
		}
	}
	if _, err := w.Write(handler.pages[index]); err != nil {
		handler.testing.Error(err)
	}
}

func TestFetchCompleteSnapshotAndRestart(t *testing.T) {
	for _, restart := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete", true: "restart"}[restart], func(t *testing.T) {
			pages := fixtures(t)
			cursor := "opaque?evil=1&/ +é"
			pages[0] = []byte(strings.ReplaceAll(string(pages[0]), "opaque-page-2", cursor))
			pages[1] = []byte(strings.ReplaceAll(string(pages[1]), "opaque-page-2", cursor))
			handler := &catalogHandler{testing: t, pages: pages, cursor: cursor, restart: restart}
			server := httptest.NewServer(handler)
			defer server.Close()
			got, err := source(server.URL + "/v1/performance-catalog").Fetch(t.Context())
			if err != nil || !reflect.DeepEqual(got, pages) {
				t.Fatalf("fetch = %s, %v", got, err)
			}
			wantCalls, wantStarts := int32(2), int32(1)
			if restart {
				wantCalls, wantStarts = 4, 2
			}
			if handler.calls.Load() != wantCalls || handler.starts.Load() != wantStarts {
				t.Fatalf("calls/starts = %d/%d", handler.calls.Load(), handler.starts.Load())
			}
		})
	}
}
func TestFetchRejectsFailuresWithoutPartialOutput(t *testing.T) {
	for _, status := range []int{401, 403, 404, 410, 429, 500, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls++
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(status)
				if _, err := io.WriteString(w, "secret-provider-body"); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			pages, err := source(server.URL).Fetch(t.Context())
			var got *StatusError
			if pages != nil || !errors.As(err, &got) || got.Code != status || strings.Contains(err.Error(), "secret") {
				t.Fatalf("unsafe error: %v", err)
			}
			want := 1
			if status == 410 {
				want = 2
			}
			if calls != want {
				t.Fatalf("requests = %d, want %d", calls, want)
			}
		})
	}
}

func TestRejectsInvalidPagesWithoutPartialOutput(t *testing.T) {
	pages := fixtures(t)
	for name, body := range map[string][]byte{"malformed": []byte(`{"page":`), "partial": pages[0], "oversized": []byte(strings.Repeat("x", (2<<20)+1)), "ambiguous": []byte(`{"page":{"nextCursor":null},"page":{"nextCursor":null}}`)} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if _, err := w.Write(body); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			got, err := source(server.URL).Fetch(t.Context())
			if err == nil || got != nil {
				t.Fatal("exposed invalid or incomplete pages")
			}
		})
	}
}

func TestRejectRedirectAndHonorCancellation(t *testing.T) {
	redirected := false
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected = true }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer server.Close()
	input := source(server.URL)
	input.Client = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return nil }}
	if pages, err := input.Fetch(t.Context()); err == nil || pages != nil || redirected {
		t.Fatal("followed credential-bearing redirect")
	}
	entered := make(chan struct{})
	cancelServer := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done() }))
	defer cancelServer.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := source(cancelServer.URL).Fetch(ctx); result <- err }()
	<-entered
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation = %v", err)
	}
}

func TestRejectsUnsafeConfigurationAndExpiredCompletion(t *testing.T) {
	for _, endpoint := range []string{"http://provider.example/catalog", "http://localhost/catalog", "https://user:password@provider.example/catalog", "https://provider.example/catalog?x=1", "https://provider.example/catalog#fragment"} {
		if _, err := source(endpoint).Fetch(t.Context()); err == nil || strings.Contains(err.Error(), "password") {
			t.Fatalf("unsafe configuration error: %v", err)
		}
	}
	pages := fixtures(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		index := 0
		if r.URL.RawQuery != "" {
			index = 1
		}
		if _, err := w.Write(pages[index]); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	input := source(server.URL)
	input.Clock = func() time.Time { return time.Date(2026, 10, 22, 13, 0, 0, 0, time.UTC) }
	if got, err := input.Fetch(t.Context()); err == nil || got != nil {
		t.Fatal("exposed stale snapshot")
	}
	input = source(server.URL)
	observations := 0
	input.Clock = func() time.Time {
		observations++
		return time.Date(2026, 10, 22, 11+observations, 0, 0, 0, time.UTC)
	}
	if got, err := input.Fetch(t.Context()); err == nil || got != nil {
		t.Fatal("exposed snapshot that expired during validation")
	}
	input = source(server.URL)
	input.Expected.PrincipalID = "other"
	if got, err := input.Fetch(t.Context()); err == nil || got != nil {
		t.Fatal("accepted another principal")
	}
}
