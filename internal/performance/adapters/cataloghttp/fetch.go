// Package cataloghttp fetches authorized catalog pages from one reviewed endpoint.
package cataloghttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/stanimirivanov/argus/internal/performance/adapters/perfeng"
)

// Source defines independently reviewed authority and transport inputs. Client
// and Clock are optional trusted embedding/test dependencies; nil uses defaults.
// Fetch clones the client and never follows redirects or uses its cookie jar.
type Source struct {
	Endpoint, Token string
	Expected        perfeng.Expectations
	Client          *http.Client
	Clock           func() time.Time
}

// StatusError exposes only the HTTP status, never remote bodies or credentials.
// Except for one 410 restart, retries are the operator's responsibility.
type StatusError struct{ Code int }

func (err *StatusError) Error() string { return fmt.Sprintf("catalog HTTP status %d", err.Code) }

// Fetch returns complete, owned page bytes only after full validation at the
// completion instant. It grants no execution authority and publishes no state.
// The entire operation is limited to 30 seconds and 64 MiB across both attempts;
// each attempt allows 1,000 pages, each response body at most 2 MiB and 10 seconds.
func (source Source) Fetch(ctx context.Context) ([]json.RawMessage, error) {
	endpoint, client, err := source.configure()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	remaining := 64 << 20
	for attempt := range 2 {
		endpoint.RawQuery = ""
		pages, err := enumerate(ctx, client, endpoint, source.Token, &remaining)
		var status *StatusError
		if attempt == 0 && errors.As(err, &status) && status.Code == http.StatusGone {
			continue
		}
		if err != nil {
			return nil, err
		}
		clock := source.Clock
		if clock == nil {
			clock = time.Now
		}
		if err := perfeng.Validate(pages, source.Expected, clock()); err != nil {
			return nil, err
		}
		if err := freshAtReturn(pages[0], clock); err != nil {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		return pages, nil
	}

	return nil, errors.New("catalog restart exhausted")
}

// Validation can take time; do not expose a view that expired during that work.
func freshAtReturn(data []byte, clock func() time.Time) error {
	var page struct {
		Snapshot struct{ GeneratedAt, ValidUntil time.Time }
	}
	if err := json.Unmarshal(data, &page); err != nil {
		return errors.New("invalid catalog freshness metadata")
	}
	at := clock()
	if at.Before(page.Snapshot.GeneratedAt) || !at.Before(page.Snapshot.ValidUntil) {
		return errors.New("catalog freshness changed during validation")
	}

	return nil
}

func (source Source) configure() (*url.URL, *http.Client, error) {
	endpoint, err := url.Parse(source.Endpoint)
	if err != nil || endpoint.Host == "" || endpoint.User != nil || endpoint.Opaque != "" ||
		strings.ContainsAny(source.Endpoint, "?#") {
		return nil, nil, errors.New("invalid catalog endpoint")
	}
	address, addressErr := netip.ParseAddr(endpoint.Hostname())
	if endpoint.Scheme != "https" && !(endpoint.Scheme == "http" && addressErr == nil && address.IsLoopback()) {
		return nil, nil, errors.New("catalog requires HTTPS or literal loopback HTTP")
	}
	if !regexp.MustCompile(`^[A-Za-z0-9._~+/-]+=*$`).MatchString(source.Token) {
		return nil, nil, errors.New("catalog bearer credential is missing or invalid")
	}
	for _, value := range []string{source.Expected.Issuer, source.Expected.PrincipalIssuer,
		source.Expected.PrincipalID, source.Expected.ProducerID, source.Expected.ProducerVersion} {
		if strings.TrimSpace(value) == "" {
			return nil, nil, errors.New("independent catalog expectations are required")
		}
	}
	client := http.Client{Timeout: 10 * time.Second}
	if source.Client != nil {
		client = *source.Client
		if client.Timeout <= 0 || client.Timeout > 10*time.Second {
			client.Timeout = 10 * time.Second
		}
	}
	client.Jar = nil
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errors.New("catalog redirect rejected")
	}

	return endpoint, &client, nil
}

func enumerate(ctx context.Context, client *http.Client, endpoint *url.URL, token string, remaining *int) ([]json.RawMessage, error) {
	pages := make([]json.RawMessage, 0)
	seen := map[string]bool{}
	for range 1000 {
		data, err := readPage(ctx, client, endpoint, token, min(2<<20, *remaining))
		if err != nil {
			return nil, err
		}
		*remaining -= len(data)
		pages = append(pages, data)
		var envelope struct {
			Page struct {
				NextCursor *string `json:"nextCursor"`
			} `json:"page"`
		}
		if err := json.Unmarshal(data, &envelope); err != nil {
			return nil, errors.New("invalid catalog page JSON")
		}
		if envelope.Page.NextCursor == nil {
			return pages, nil
		}
		cursor := *envelope.Page.NextCursor
		if cursor == "" || utf8.RuneCountInString(cursor) > 256 || seen[cursor] {
			return nil, errors.New("invalid or repeated catalog cursor")
		}
		seen[cursor] = true
		endpoint.RawQuery = url.Values{"cursor": {cursor}}.Encode()
	}

	return nil, errors.New("catalog page limit exceeded")
}

func readPage(ctx context.Context, client *http.Client, endpoint *url.URL, token string, limit int) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, errors.New("invalid catalog request")
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("catalog request failed or timed out")
	}
	if response.StatusCode != http.StatusOK {
		if err := response.Body.Close(); err != nil {
			return nil, errors.New("close catalog error response")
		}
		return nil, &StatusError{Code: response.StatusCode}
	}
	data, readErr := io.ReadAll(io.LimitReader(response.Body, int64(limit)+1))
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("read catalog response failed or timed out")
	}
	if len(data) > limit {
		return nil, errors.New("catalog response exceeds byte budget")
	}

	return data, nil
}
