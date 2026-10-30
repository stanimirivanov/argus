package perfeng

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
)

func analysisS3Fixture(t *testing.T, transport integrationRoundTrip) (*AnalysisS3, PublicationAdmission, NativeReview, []byte) {
	t.Helper()
	f := conformanceFixture(t)
	f.publication.Data.Artifact.URI = "s3://perfeng-artifacts/reports/report.json"
	admitted, review, raw := conformanceInputs(t, f)
	config := AnalysisS3Config{Region: "us-east-1", Bucket: "perfeng-artifacts", Prefix: "reports/", Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
		return aws.Credentials{AccessKeyID: "TESTKEY", SecretAccessKey: "test-secret"}, nil
	}), Client: &http.Client{Transport: transport}}
	source, err := NewAnalysisS3(config)
	if err != nil {
		t.Fatal(err)
	}

	return source, admitted, review, raw
}

func TestAnalysisS3SignedExactReportAndOwnedEvidence(t *testing.T) {
	var raw []byte
	calls := 0
	source, admitted, review, wire := analysisS3Fixture(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != http.MethodGet || r.URL.Scheme != "https" || r.URL.Host != "perfeng-artifacts.s3.us-east-1.amazonaws.com" || r.URL.Path != "/reports/report.json" || r.URL.RawQuery != "x-id=GetObject" {
			t.Error("unexpected object access", r.URL)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=TESTKEY/") || r.Header.Get("Cookie") != "" || r.Body != nil {
			t.Error("invalid signed read")
		}
		deadline, ok := r.Context().Deadline()
		if !ok || time.Until(deadline) > 10*time.Second {
			t.Error("missing deadline")
		}

		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(bytes.NewReader(raw))}, nil
	})
	raw = wire
	checked, err := source.Fetch(context.Background(), admitted, review)
	if err != nil || calls != 1 || !bytes.Equal(checked.Data().ReportBytes, raw) || checked.Data().Admission.Data().SHA256 != admitted.Data().SHA256 {
		t.Fatal("report verification failed", err, calls)
	}
	copy := checked.Data()
	copy.ReportBytes[0] = '!'
	if !bytes.Equal(checked.Data().ReportBytes, raw) {
		t.Fatal("aliased report")
	}
}

func TestAnalysisS3RejectsBeforeObjectAccess(t *testing.T) {
	for _, mode := range []string{"bucket", "prefix", "sibling", "traversal", "encoded-traversal", "scheme", "oversized", "review", "transplanted", "admission", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			source, _, _, _ := analysisS3Fixture(t, func(*http.Request) (*http.Response, error) {
				t.Error("invalid scope reached network")
				return nil, errors.New("unexpected")
			})
			f := conformanceFixture(t)
			f.publication.Data.Artifact.URI = "s3://perfeng-artifacts/reports/report.json"
			switch mode {
			case "bucket":
				f.publication.Data.Artifact.URI = "s3://other-artifacts/reports/report.json"
			case "prefix":
				f.publication.Data.Artifact.URI = "s3://perfeng-artifacts/other/report.json"
			case "sibling":
				f.publication.Data.Artifact.URI = "s3://perfeng-artifacts/reports-other/report.json"
			case "traversal":
				f.publication.Data.Artifact.URI = "s3://perfeng-artifacts/reports/../report.json"
			case "encoded-traversal":
				f.publication.Data.Artifact.URI = "s3://perfeng-artifacts/reports/%2E%2E/report.json"
			case "scheme":
				f.publication.Data.Artifact.URI = "https://perfeng-artifacts/reports/report.json"
			}
			admitted, review, _ := conformanceInputs(t, f)
			switch mode {
			case "oversized":
				admitted = oversizedAnalysisS3Admission(t, admitted)
			case "review":
				review = NativeReview{}
			case "transplanted":
				other := conformanceFixture(t)
				other.bundle.Native.Workload.ID += "-other"
				other.publication.Data.Native.Workload.ID += "-other"
				_, review, _ = conformanceInputs(t, other)
			case "admission":
				admitted = PublicationAdmission{}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancelled" {
				cancel()
			}
			checked, err := source.Fetch(ctx, admitted, review)
			if err == nil || checked.Data().SHA256 != "" {
				t.Fatal("invalid authority accepted")
			}
		})
	}
}

func oversizedAnalysisS3Admission(t *testing.T, admitted PublicationAdmission) PublicationAdmission {
	t.Helper()
	// Package-local mutation simulates a publication with an oversized claim;
	// the byte-limit preflight must reject it before requesting object contents.
	p, err := DecodePublication(admitted.data.Payload)
	if err != nil {
		t.Fatal(err)
	}
	p.Data.Artifact.SizeBytes = "1048577"
	admitted.data.Payload, err = json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}

	return admitted
}

func TestAnalysisS3FailuresAreBoundedSafeAndNotRetried(t *testing.T) {
	for _, mode := range []string{"403", "404", "503", "redirect", "media", "oversized", "corrupt", "read", "close"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			var raw []byte
			body := &analysisS3ClosingBody{Reader: &publicationFailingBody{readErr: errors.New("private read failure")}}
			source, admitted, review, wire := analysisS3Fixture(t, func(r *http.Request) (*http.Response, error) {
				calls++
				if mode == "close" {
					body.Reader = bytes.NewReader(raw)
				}

				return analysisS3FailureResponse(mode, raw, body), nil
			})
			raw = wire
			checked, err := source.Fetch(context.Background(), admitted, review)
			if err == nil || checked.Data().SHA256 != "" || strings.Contains(err.Error(), "private") || calls > 1 {
				t.Fatal("unsafe failure", err, calls)
			}
			if (mode == "read" || mode == "close") && body.closed != 1 {
				t.Fatal("body leaked", body.closed)
			}
		})
	}
}

func analysisS3FailureResponse(mode string, raw []byte, body *analysisS3ClosingBody) *http.Response {
	response := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(bytes.NewReader(raw))}
	switch mode {
	case "403":
		response.StatusCode = 403
	case "404":
		response.StatusCode = 404
	case "503":
		response.StatusCode = 503
	case "redirect":
		response.StatusCode = 302
		response.Header.Set("Location", "https://untrusted.example/report")
	case "media":
		response.Header.Set("Content-Type", "text/plain")
	case "oversized":
		response.Body = io.NopCloser(strings.NewReader(strings.Repeat(" ", AnalysisMaximumBytes+1)))
	case "corrupt":
		response.Body = io.NopCloser(strings.NewReader("{}"))
	case "read":
		response.Body = body
	case "close":
		body.closeErr = errors.New("private close failure")
		response.Body = body
	}

	return response
}

func TestAnalysisS3ConfigurationAndClientIsolation(t *testing.T) {
	source, _, _, _ := analysisS3Fixture(t, nil)
	if source == nil {
		t.Fatal("missing client")
	}
	credentials := aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) { return aws.Credentials{}, nil })
	for _, config := range []AnalysisS3Config{
		{}, {Region: "bad", Bucket: "perfeng-artifacts", Prefix: "reports/", Credentials: credentials},
		{Region: "us-east-1", Bucket: "127.0.0.1", Prefix: "reports/", Credentials: credentials},
		{Region: "us-east-1", Bucket: "perfeng-artifacts", Prefix: "", Credentials: credentials},
		{Region: "us-east-1", Bucket: "perfeng-artifacts", Prefix: "reports", Credentials: credentials},
		{Region: "us-east-1", Bucket: "perfeng-artifacts", Prefix: "reports/../", Credentials: credentials},
	} {
		if got, err := NewAnalysisS3(config); err == nil || got != nil {
			t.Fatal("invalid config accepted")
		}
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: time.Hour, Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return nil }}
	got, err := NewAnalysisS3(AnalysisS3Config{Region: "us-east-1", Bucket: "perfeng-artifacts", Prefix: "reports/", Credentials: credentials, Client: client})
	if err != nil {
		t.Fatal(err)
	}
	cloned, ok := got.client.Options().HTTPClient.(*http.Client)
	if !ok || cloned == client || cloned.Jar != nil || cloned.Timeout != 10*time.Second || client.Jar != jar || client.Timeout != time.Hour || cloned.CheckRedirect(nil, nil) == nil {
		t.Fatal("client isolation failed")
	}
}

type analysisS3ClosingBody struct {
	io.Reader
	closed   int
	closeErr error
}

func (b *analysisS3ClosingBody) Close() error { b.closed++; return b.closeErr }

func TestAnalysisS3CredentialAndExchangeFailures(t *testing.T) {
	for _, mode := range []string{"credentials", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			source, admitted, review, _ := analysisS3Fixture(t, func(r *http.Request) (*http.Response, error) {
				cancel()
				<-r.Context().Done()

				return nil, r.Context().Err()
			})
			if mode == "credentials" {
				var err error
				source, err = NewAnalysisS3(AnalysisS3Config{Region: "us-east-1", Bucket: "perfeng-artifacts", Prefix: "reports/", Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
					return aws.Credentials{}, errors.New("private credentials")
				})})
				if err != nil {
					t.Fatal(err)
				}
			}
			checked, err := source.Fetch(ctx, admitted, review)
			if err == nil || checked.Data().SHA256 != "" || strings.Contains(err.Error(), "private") {
				t.Fatal("unsafe failure", err)
			}
			if mode == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		})
	}
}
