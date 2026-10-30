package perfeng

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func TestPublicationHTTPPinsReadContract(t *testing.T) {
	raw, err := os.ReadFile("snapshot/publication/read-openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != "0bdafe32dec51a51b42a512d5a3947356aa26a5c958768ec8cab68327d466556" {
		t.Fatal("published API 1.1 snapshot changed")
	}
}

func publicationHTTPIdentityFixture(t *testing.T, id string) publicationAdmissionFixtureData {
	t.Helper()
	f := publicationAdmissionFixture(t)
	original := lifecycleReply(t, f.frozen)
	original.Data.Context.Request.ID = id
	var err error
	f.frozen, err = EncodeIntegration(original)
	if err != nil {
		t.Fatal(err)
	}
	reply := lifecycleReply(t, f.retained.Data().Payload)
	reply.Data.Context = original.Data.Context
	f.retained, err = DispatchOutcome(f.frozen, reply, f.provider)
	if err != nil {
		t.Fatal(err)
	}
	publication, err := DecodePublication(f.wire)
	if err != nil {
		t.Fatal(err)
	}
	publication.Data.Context = original.Data.Context
	f.wire, err = EncodePublication(publication)
	if err != nil {
		t.Fatal(err)
	}
	f.wire = append(append([]byte(" \n"), f.wire...), '\n')

	return f
}

func publicationHTTPHandler(t *testing.T, f publicationAdmissionFixtureData, id string, calls *atomic.Int32) http.Handler {
	t.Helper()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		escaped := url.PathEscape(id)
		if id == ".." {
			escaped = "%2E%2E"
		}
		if r.Method != http.MethodGet || r.URL.EscapedPath() != "/integration/v1/requests/"+escaped+"/analysis" || r.URL.Query().Get("issuer") != "https://argus.example/identity" {
			t.Error("request identity changed")
		}
		body, readErr := io.ReadAll(r.Body)
		if readErr != nil || len(body) != 0 || r.Header.Get("Authorization") != "Bearer secret" || r.Header.Get("Accept") != "application/json" {
			t.Error("invalid authenticated GET")
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if _, err := w.Write(f.wire); err != nil {
			t.Error(err)
		}
	})
}

func TestPublicationHTTPPreservesBytesAndOpaqueIdentity(t *testing.T) {
	for _, id := range []string{"request-1", "request/analysis", "..", "opaque?#%λ"} {
		t.Run(id, func(t *testing.T) {
			f := publicationHTTPIdentityFixture(t, id)
			var calls atomic.Int32
			server := httptest.NewServer(publicationHTTPHandler(t, f, id, &calls))
			defer server.Close()
			source, _ := integrationHTTPFixture(t)
			source.Endpoint = server.URL + "/integration/v1/requests"
			bound, err := source.LookupPublication(context.Background(), f.frozen, f.retained)
			if err != nil || calls.Load() != 1 || !bytes.Equal(bound.Data().Payload, f.wire) || bound.Data().Admission.Data().Evidence.Fingerprint() != f.retained.Fingerprint() {
				t.Fatal("publication lost bytes/admission", err, calls.Load())
			}
		})
	}
}

func TestPublicationHTTPRejectsBeforeIO(t *testing.T) {
	for _, mode := range []string{"configuration", "principal", "frozen", "settlement", "forged", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			f := publicationAdmissionFixture(t)
			source, _ := integrationHTTPFixture(t)
			source.Client = &http.Client{Transport: integrationRoundTrip(func(*http.Request) (*http.Response, error) {
				t.Error("invalid admission reached network")
				return nil, errors.New("unexpected")
			})}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch mode {
			case "configuration":
				source.Token = "invalid token"
			case "principal":
				source.Principal.ID += "-other"
			case "frozen":
				f.frozen = []byte("{}")
			case "settlement":
				f.retained = requestintent.Outcome{}
			case "forged":
				f.retained = changedAdmission(t, f.retained)
			case "cancelled":
				cancel()
			}
			bound, err := source.LookupPublication(ctx, f.frozen, f.retained)
			if err == nil || len(bound.Data().Payload) != 0 {
				t.Fatal("invalid preflight accepted")
			}
			if mode == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		})
	}
}

func TestPublicationHTTPStatusAndRetryHints(t *testing.T) {
	for _, code := range []int{401, 403, 404, 429, 503} {
		for _, retry := range []string{"7", "0", "invalid", "999999999999999999999"} {
			f := publicationAdmissionFixture(t)
			source, _ := integrationHTTPFixture(t)
			calls := 0
			source.Client = &http.Client{Transport: integrationRoundTrip(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: code, Header: http.Header{"Retry-After": {retry}}, Body: io.NopCloser(strings.NewReader("private provider details"))}, nil
			})}
			bound, err := source.LookupPublication(context.Background(), f.frozen, f.retained)
			var status *IntegrationHTTPStatusError
			if !errors.As(err, &status) || status.Code != code || calls != 1 || len(bound.Data().Payload) != 0 || strings.Contains(err.Error(), "private") {
				t.Fatal("unsafe status handling", err)
			}
			expected := time.Duration(0)
			if (code == 429 || code == 503) && retry == "7" {
				expected = 7 * time.Second
			}
			if status.RetryAfter != expected {
				t.Fatal("incorrect retry hint", status)
			}
		}
	}
}

func TestPublicationHTTPRejectsUntrustedResponses(t *testing.T) {
	for _, mode := range []string{"malformed", "oversized", "media", "provider", "run", "context", "transport"} {
		t.Run(mode, func(t *testing.T) {
			f := publicationAdmissionFixture(t)
			source, _ := integrationHTTPFixture(t)
			media := "application/json"
			switch mode {
			case "malformed":
				f.wire = []byte("{")
			case "oversized":
				f.wire = []byte(strings.Repeat(" ", PublicationMaximumBytes+1))
			case "media":
				media = "text/plain"
			case "provider", "run", "context":
				f.wire = substitutedPublicationHTTPResponse(t, f.wire, mode)
			}
			source.Client = &http.Client{Transport: integrationRoundTrip(func(*http.Request) (*http.Response, error) {
				if mode == "transport" {
					return nil, errors.New("private transport details")
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {media}}, Body: io.NopCloser(bytes.NewReader(f.wire))}, nil
			})}
			bound, err := source.LookupPublication(context.Background(), f.frozen, f.retained)
			if !errors.Is(err, ErrPublication) || len(bound.Data().Payload) != 0 || strings.Contains(err.Error(), "private") {
				t.Fatal("untrusted response accepted", err)
			}
		})
	}
}

func TestPublicationHTTPDisablesRedirectsAndCookies(t *testing.T) {
	f := publicationAdmissionFixture(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Cookie") != "" {
			t.Error("ambient credentials leaked")
		}
		http.Redirect(w, r, "/artifact", http.StatusFound)
	}))
	defer server.Close()
	source, _ := integrationHTTPFixture(t)
	source.Endpoint = server.URL + "/integration/v1/requests"
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	jar.SetCookies(target, []*http.Cookie{{Name: "ambient", Value: "secret"}})
	source.Client = &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { t.Error("caller redirect policy used"); return nil }}
	_, err = source.LookupPublication(context.Background(), f.frozen, f.retained)
	if !errors.Is(err, ErrPublication) || calls.Load() != 1 || source.Client.Jar != jar {
		t.Fatal("redirect/client isolation failed", err, calls.Load())
	}
}

type publicationFailingBody struct {
	closed            int
	readErr, closeErr error
}

func (b *publicationFailingBody) Read([]byte) (int, error) { return 0, b.readErr }
func (b *publicationFailingBody) Close() error             { b.closed++; return b.closeErr }

func TestPublicationHTTPClosesFailedBodies(t *testing.T) {
	for _, mode := range []string{"read", "close", "status-close"} {
		t.Run(mode, func(t *testing.T) {
			f := publicationAdmissionFixture(t)
			source, _ := integrationHTTPFixture(t)
			body := &publicationFailingBody{readErr: io.EOF}
			code := 200
			switch mode {
			case "read":
				body.readErr = errors.New("private read failure")
			case "close":
				body.closeErr = errors.New("private close failure")
			case "status-close":
				body.closeErr = errors.New("private close failure")
				code = 503
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			source.Client = &http.Client{Timeout: time.Hour, Transport: integrationRoundTrip(func(r *http.Request) (*http.Response, error) {
				deadline, ok := r.Context().Deadline()
				if !ok || time.Until(deadline) > 10*time.Second {
					t.Error("missing bounded deadline")
				}

				return &http.Response{StatusCode: code, Header: http.Header{"Content-Type": {"application/json"}}, Body: body}, nil
			})}
			_, err := source.LookupPublication(ctx, f.frozen, f.retained)
			if err == nil || strings.Contains(err.Error(), "private") {
				t.Fatal("unsafe failure", err)
			}
			if body.closed != 1 {
				t.Fatal("body not closed once", body.closed)
			}
		})
	}
}

func substitutedPublicationHTTPResponse(t *testing.T, raw []byte, mode string) []byte {
	t.Helper()
	p, err := DecodePublication(raw)
	if err != nil {
		t.Fatal(err)
	}
	switch mode {
	case "provider":
		p.Source += "/other"
		p.Data.Run.Issuer = p.Source
	case "run":
		p.Data.Run.ID += "-other"
	case "context":
		p.Data.Context.Attempt.ID += "-other"
	}
	raw, err = EncodePublication(p)
	if err != nil {
		t.Fatal(err)
	}

	return raw
}

func TestPublicationHTTPCancellationDuringExchange(t *testing.T) {
	f := publicationAdmissionFixture(t)
	source, _ := integrationHTTPFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	source.Client = &http.Client{Transport: integrationRoundTrip(func(r *http.Request) (*http.Response, error) {
		cancel()
		<-r.Context().Done()

		return nil, r.Context().Err()
	})}
	bound, err := source.LookupPublication(ctx, f.frozen, f.retained)
	if !errors.Is(err, context.Canceled) || len(bound.Data().Payload) != 0 {
		t.Fatal("cancellation lost", err)
	}
}
