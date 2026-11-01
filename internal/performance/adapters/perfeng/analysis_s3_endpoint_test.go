package perfeng

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
)

func endpointAnalysisConfig(endpoint string, pathStyle bool, client *http.Client) AnalysisS3Config {
	return AnalysisS3Config{Region: "us-east-1", Bucket: "perfeng-artifacts", Prefix: "reports/", Endpoint: endpoint, UsePathStyle: pathStyle, Client: client,
		Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
			return aws.Credentials{AccessKeyID: "TESTKEY", SecretAccessKey: "test-secret"}, nil
		})}
}

func TestAnalysisEndpointRejectsAmbiguousOriginsAndTLSBypass(t *testing.T) {
	for _, endpoint := range []string{"http://localhost:9443", "https://user:private@localhost", "https://localhost/proxy", "https://localhost/%2f", "https://localhost?", "https://localhost#", "https://localhost?key=private", "https://localhost#fragment", "https://localhost:", "https://localhost:0", "https://localhost:65536", "https://localhost:bad", "https://", "//localhost", " https://localhost", "https://bad_host", "https://[bad-host]", "https://-host", "https://host..example", "https://[fe80::1%25eth0]"} {
		t.Run(endpoint, func(t *testing.T) {
			source, err := NewAnalysisS3(endpointAnalysisConfig(endpoint, true, nil))
			if source != nil || !errors.Is(err, ErrAnalysisAccess) || strings.Contains(err.Error(), "private") {
				t.Fatal("unsafe endpoint admitted", err)
			}
		})
	}
	for _, config := range []*tls.Config{{InsecureSkipVerify: true}, {ServerName: "different.example"}} {
		source, err := NewAnalysisS3(endpointAnalysisConfig("https://localhost:9443", true, &http.Client{Transport: &http.Transport{TLSClientConfig: config}}))
		if source != nil || !errors.Is(err, ErrAnalysisAccess) {
			t.Fatal("TLS identity bypass admitted", err)
		}
	}
}

func TestAnalysisEndpointAddressingAndEscapedKey(t *testing.T) {
	for _, pathStyle := range []bool{true, false} {
		t.Run(map[bool]string{true: "path", false: "virtual"}[pathStyle], func(t *testing.T) {
			testAnalysisEndpointAddressing(t, pathStyle)
		})
	}
}

func testAnalysisEndpointAddressing(t *testing.T, pathStyle bool) {
	f := conformanceFixture(t)
	f.publication.Data.Artifact.URI = "s3://perfeng-artifacts/reports/space%20name%2Bplus.json"
	admitted, review, raw := conformanceInputs(t, f)
	calls := 0
	client := &http.Client{Transport: integrationRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		host, path := "perfeng-artifacts.objects.example:9443", "/reports/space%20name%2Bplus.json"
		if pathStyle {
			host, path = "objects.example:9443", "/perfeng-artifacts"+path
		}
		if r.Method != "GET" || r.URL.Scheme != "https" || r.URL.Host != host || r.URL.EscapedPath() != path || r.URL.RawQuery != "x-id=GetObject" {
			t.Errorf("unexpected destination %s", r.URL)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=TESTKEY/") || !strings.Contains(r.Header.Get("Authorization"), "/us-east-1/s3/aws4_request") || r.Header.Get("Cookie") != "" {
			t.Error("signing or credential boundary lost")
		}

		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(bytes.NewReader(raw))}, nil
	})}
	source, err := NewAnalysisS3(endpointAnalysisConfig("https://objects.example:9443/", pathStyle, client))
	if err != nil {
		t.Fatal(err)
	}
	checked, err := source.Fetch(t.Context(), admitted, review)
	if err != nil || calls != 1 || !bytes.Equal(checked.Data().ReportBytes, raw) {
		t.Fatal("custom endpoint failed", err, calls)
	}
}

func TestAnalysisEndpointVerifiedTLSAndRedirectRefusal(t *testing.T) {
	for _, mode := range []string{"trusted", "untrusted", "redirect", "foreign-bucket", "foreign-prefix"} {
		t.Run(mode, func(t *testing.T) { testAnalysisEndpointTLS(t, mode) })
	}
}
func testAnalysisEndpointTLS(t *testing.T, mode string) {
	f := conformanceFixture(t)
	f.publication.Data.Artifact.URI = "s3://perfeng-artifacts/reports/report.json"
	admitted, review, raw := conformanceInputs(t, f)
	var calls, foreign atomic.Int32
	target := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { foreign.Add(1) }))
	defer target.Close()
	calls.Store(0)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") || r.URL.Path != "/perfeng-artifacts/reports/report.json" {
			t.Error("unsigned or unexpected TLS request")
		}
		if mode == "redirect" {
			http.Redirect(w, r, target.URL+"/private", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(raw); err != nil {
			t.Error(err)
		}
	}))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	defer server.Close()
	roots := x509.NewCertPool()
	if mode != "untrusted" {
		roots.AddCert(server.Certificate())
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}
	defer transport.CloseIdleConnections()
	source, err := NewAnalysisS3(endpointAnalysisConfig(server.URL, true, &http.Client{Transport: transport}))
	if err != nil {
		t.Fatal(err)
	}
	bound, native := admitted, review
	if strings.HasPrefix(mode, "foreign-") {
		changed := f
		changed.publication.Data.Artifact.URI = "s3://other-artifacts/reports/report.json"
		if mode == "foreign-prefix" {
			changed.publication.Data.Artifact.URI = "s3://perfeng-artifacts/reports-other/report.json"
		}
		bound, native, _ = conformanceInputs(t, changed)
	}
	checked, err := source.Fetch(t.Context(), bound, native)
	assertAnalysisEndpointTLS(t, mode, checked, err, calls.Load(), foreign.Load())
}
func assertAnalysisEndpointTLS(t *testing.T, mode string, checked ReviewedAnalysis, err error, calls, foreign int32) {
	t.Helper()
	if mode == "trusted" {
		if err != nil || checked.Data().SHA256 == "" {
			t.Fatal("trusted TLS failed", err)
		}
	} else if !errors.Is(err, ErrAnalysisAccess) || checked.Data().SHA256 != "" {
		t.Fatal("unsafe TLS/scope accepted", err)
	}
	expected := int32(0)
	if mode == "trusted" || mode == "redirect" {
		expected = 1
	}
	if calls != expected || foreign != 0 {
		t.Fatal("unexpected retry, redirect or foreign read", calls, foreign)
	}
}
