package perfeng

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type integrationRoundTrip func(*http.Request) (*http.Response, error)

func (f integrationRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func integrationHTTPFixture(t *testing.T) (IntegrationHTTP, []byte) {
	t.Helper()
	raw := integrationExchange(t, "completed")[0]
	message, err := DecodeIntegration(raw)
	if err != nil {
		t.Fatal(err)
	}

	return IntegrationHTTP{Endpoint: "https://provider.example/integration/v1/requests", Token: "secret", Provider: "https://provider.example/identity", Principal: message.Data.Context.Principal}, raw
}

func integrationHTTPHandler(t *testing.T, raw []byte, messages []json.RawMessage, rejected bool, calls *atomic.Int32) http.Handler {
	t.Helper()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer secret" || r.Header.Get("Accept") != "application/json" {
			t.Error("missing authentication or accept header")
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		status, response := http.StatusAccepted, messages[1]
		if r.Method == http.MethodGet {
			if len(body) != 0 || r.URL.Path != "/integration/v1/requests/request-1" || r.URL.Query().Get("issuer") != "https://argus.example/identity" {
				t.Error("lookup changed identity or sent a body")
			}
			status, response = http.StatusOK, messages[len(messages)-1]
		} else if r.Method != http.MethodPost || !bytes.Equal(body, raw) || r.Header.Get("Content-Type") != "application/json" {
			t.Error("submission changed frozen bytes")
		}
		if rejected {
			status = http.StatusUnprocessableEntity
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if _, err := w.Write(response); err != nil {
			t.Error(err)
		}
	})
}

func TestIntegrationHTTPPreservesSubmissionReplayAndScopedLookup(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"completed", "rejected"} {
		t.Run(name, func(t *testing.T) {
			source, _ := integrationHTTPFixture(t)
			messages := integrationExchange(t, name)
			raw := append([]byte(" \n"), messages[0]...)
			var calls atomic.Int32
			server := httptest.NewServer(integrationHTTPHandler(t, raw, messages, name == "rejected", &calls))
			defer server.Close()
			source.Endpoint = server.URL + "/integration/v1/requests"
			first, err := source.Submit(t.Context(), raw)
			if err != nil {
				t.Fatal(err)
			}
			again, err := source.Submit(t.Context(), raw)
			if err != nil || !reflect.DeepEqual(first, again) {
				t.Fatal("replay changed evidence", err)
			}
			if _, err := source.Lookup(t.Context(), raw); err != nil || calls.Load() != 3 {
				t.Fatal("lookup failed or hidden retry", err, calls.Load())
			}
		})
	}
}

func TestIntegrationHTTPRejectsUntrustedResponses(t *testing.T) {
	t.Parallel()
	accepted := integrationExchange(t, "completed")[1]
	wrongRun, err := DecodeIntegration(accepted)
	if err != nil {
		t.Fatal(err)
	}
	wrongRun.Data.Run.Issuer = "https://other.example/identity"
	wrongRunBytes, err := EncodeIntegration(wrongRun)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, body string
		status     int
	}{
		{"provider", strings.ReplaceAll(string(accepted), "https://provider.example/identity", "https://other.example/identity"), 202},
		{"context", strings.ReplaceAll(string(accepted), "attempt-1", "attempt-other"), 202},
		{"run issuer", string(wrongRunBytes), 202},
		{"wrong kind", string(accepted), 422},
		{"malformed", "private remote diagnostic", 202},
		{"oversize", strings.Repeat(" ", IntegrationMaximumBytes+1), 202},
		{"unauthenticated", "private remote diagnostic", 401},
		{"forbidden", "private remote diagnostic", 403},
		{"missing", "private remote diagnostic", 404},
		{"conflict", "private remote diagnostic", 409},
		{"rate limited", "private remote diagnostic", 429},
		{"unavailable", "private remote diagnostic", 503},
	} {
		t.Run(test.name, func(t *testing.T) {
			source, raw := integrationHTTPFixture(t)
			source.Client = &http.Client{Transport: integrationRoundTrip(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: test.status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(test.body))}, nil
			})}
			got, err := source.Submit(t.Context(), raw)
			if err == nil || !reflect.DeepEqual(got, IntegrationMessage{}) || strings.Contains(err.Error(), "private") {
				t.Fatal("untrusted response exposed evidence or diagnostics", err)
			}
			if test.status >= 400 && test.status != 422 {
				var status *IntegrationHTTPStatusError
				if !errors.As(err, &status) || status.Code != test.status {
					t.Fatal("lost status classification", err)
				}
			}
		})
	}
}

func TestIntegrationHTTPConstrainsCredentialsAndRedirects(t *testing.T) {
	t.Parallel()
	var leaked atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { leaked.Add(1) }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "" {
			t.Error("client cookie jar leaked")
		}
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	source, raw := integrationHTTPFixture(t)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	jar.SetCookies(endpoint, []*http.Cookie{{Name: "private", Value: "cookie"}})
	source.Endpoint = server.URL + "/integration/v1/requests"
	source.Client = &http.Client{Jar: jar}
	if _, err := source.Submit(t.Context(), raw); err == nil || leaked.Load() != 0 || source.Client.Jar != jar || source.Client.CheckRedirect != nil {
		t.Fatal("redirect followed or embedding client mutated", err)
	}
	for _, endpoint := range []string{"http://provider.example/integration/v1/requests", server.URL + "/wrong", source.Endpoint + "?token=secret", "https://user:password@provider.example/integration/v1/requests"} {
		source.Endpoint = endpoint
		if _, err := source.Submit(t.Context(), raw); !errors.Is(err, ErrIntegration) {
			t.Fatal("unsafe endpoint accepted", err)
		}
	}
}

func TestIntegrationHTTPBoundsCallsAndEscapesOpaqueIdentities(t *testing.T) {
	t.Parallel()
	source, raw := integrationHTTPFixture(t)
	for _, id := range []string{"opaque/with?query#fragment", ".", ".."} {
		request, err := DecodeIntegration(raw)
		if err != nil {
			t.Fatal(err)
		}
		request.Data.Context.Request.ID = id
		input, err := EncodeIntegration(request)
		if err != nil {
			t.Fatal(err)
		}
		source.Client = &http.Client{Timeout: time.Hour, Transport: integrationRoundTrip(func(r *http.Request) (*http.Response, error) {
			deadline, ok := r.Context().Deadline()
			if !ok || time.Until(deadline) > 10*time.Second || strings.TrimPrefix(r.URL.Path, "/integration/v1/requests/") != id || r.URL.Query().Get("issuer") != request.Source {
				t.Error("unbounded call or changed lookup")
			}

			return nil, errors.New("private network error")
		})}
		if _, err := source.Lookup(t.Context(), input); err == nil || strings.Contains(err.Error(), "private") {
			t.Fatal("transport diagnostic leak", err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := source.Submit(ctx, raw); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
	source.Principal.ID = "unapproved"
	if _, err := source.Submit(t.Context(), raw); !errors.Is(err, ErrIntegration) {
		t.Fatal("wrong principal allowed", err)
	}
}
