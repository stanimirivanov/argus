package perfeng

import (
	"context"
	"errors"
	"io"
	"math/big"
	"mime"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// ErrAnalysisAccess reports invalid or unavailable report access without exposing
// credentials, object contents or provider diagnostics. No evidence is returned.
var ErrAnalysisAccess = errors.New("analysis report access failed")

// AnalysisS3Config is protected host policy, never publication-controlled input.
// Credentials must independently grant only GetObject on Bucket/Prefix. No ambient
// credential discovery, custom endpoint, integration token or write is supported.
// Prefix is a nonempty slash-terminated directory; Region is an AWS region.
// Client is optional trusted transport configuration, cloned without cookies or
// redirects. Its transport and credential provider must honor request contexts.
type AnalysisS3Config struct {
	Region, Bucket, Prefix string
	Credentials            aws.CredentialsProvider
	Client                 *http.Client
}

// AnalysisS3 reads only admitted report objects, never candidate/baseline URIs.
// Construct with NewAnalysisS3; configuration is immutable and concurrent-safe
// when the supplied transport and credentials provider are concurrent-safe.
type AnalysisS3 struct {
	client         *s3.Client
	bucket, prefix string
}

var analysisS3Bucket = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)
var analysisS3Region = regexp.MustCompile(`^[a-z]{2}(?:-[a-z]+)+-[0-9]+$`)

// NewAnalysisS3 creates a one-attempt SigV4 client with AWS-derived HTTPS endpoints
// and at most ten seconds per call, including credentials and body consumption.
// IAM enforcement remains the host's responsibility; this adds an independent
// bucket/key-prefix check. Configuration errors return no client.
func NewAnalysisS3(config AnalysisS3Config) (*AnalysisS3, error) {
	_, ipErr := netip.ParseAddr(config.Bucket)
	if config.Credentials == nil || !analysisS3Region.MatchString(config.Region) || !analysisS3Bucket.MatchString(config.Bucket) || ipErr == nil || strings.Contains(config.Bucket, "..") || strings.Contains(config.Bucket, ".-") || strings.Contains(config.Bucket, "-.") || !strings.HasSuffix(config.Prefix, "/") || !analysisS3Key(strings.TrimSuffix(config.Prefix, "/")) {
		return nil, ErrAnalysisAccess
	}
	client := &http.Client{}
	if config.Client != nil {
		copy := *config.Client
		client = &copy
	}
	client.Jar = nil
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return errors.New("analysis redirect rejected") }
	if client.Timeout <= 0 || client.Timeout > 10*time.Second {
		client.Timeout = 10 * time.Second
	}
	sdk := s3.New(s3.Options{Region: config.Region, Credentials: config.Credentials, HTTPClient: client, Retryer: aws.NopRetryer{}})

	return &AnalysisS3{client: sdk, bucket: config.Bucket, prefix: config.Prefix}, nil
}

// Fetch verifies admission/review authority before one signed GET, bounds the
// decoded report to 1 MiB and the whole call to ten seconds, then checks exact
// bytes and reviewed conformance. It returns owned evidence or a safe error,
// preserving context cancellation. Reads/retries/writes are never delegated to
// report URI claims. The caller owns any subsequent retry or durable retention.
func (source *AnalysisS3) Fetch(ctx context.Context, admitted PublicationAdmission, review NativeReview) (ReviewedAnalysis, error) {
	if err := ctx.Err(); err != nil {
		return ReviewedAnalysis{}, err
	}
	if source == nil || source.client == nil || review.data.SHA256 == "" || !validNativeReview(review.Data().Bundle, admitted, review.data.Bundle.ReviewedBy) {
		return ReviewedAnalysis{}, ErrAnalysisAccess
	}
	publication, err := DecodePublication(admitted.Data().Payload)
	if err != nil {
		return ReviewedAnalysis{}, ErrAnalysisAccess
	}
	artifact := publication.Data.Artifact
	uri, err := url.Parse(artifact.URI)
	if err != nil || uri.Scheme != "s3" || uri.Host != source.bucket || uri.User != nil || uri.RawQuery != "" || uri.ForceQuery || uri.Fragment != "" || uri.Opaque != "" || !strings.HasPrefix(uri.Path, "/") {
		return ReviewedAnalysis{}, ErrAnalysisAccess
	}
	key := strings.TrimPrefix(uri.Path, "/")
	size, validSize := new(big.Rat).SetString(artifact.SizeBytes.String())
	if !analysisS3Key(key) || !strings.HasPrefix(key, source.prefix) || len(key) <= len(source.prefix) || !validSize || !size.IsInt() || size.Sign() <= 0 || size.Cmp(new(big.Rat).SetInt64(AnalysisMaximumBytes)) > 0 {
		return ReviewedAnalysis{}, ErrAnalysisAccess
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	response, err := source.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(source.bucket), Key: aws.String(key)})
	if err != nil {
		return ReviewedAnalysis{}, analysisAccessError(ctx)
	}
	raw, readErr := io.ReadAll(io.LimitReader(response.Body, AnalysisMaximumBytes+1))
	closeErr := response.Body.Close()
	if ctx.Err() != nil {
		return ReviewedAnalysis{}, ctx.Err()
	}
	media, _, mediaErr := mime.ParseMediaType(aws.ToString(response.ContentType))
	if readErr != nil || closeErr != nil || mediaErr != nil || media != "application/json" || len(raw) > AnalysisMaximumBytes {
		return ReviewedAnalysis{}, ErrAnalysisAccess
	}
	checked, err := VerifyReviewedAnalysis(ctx, admitted, review, raw)
	if err != nil {
		return ReviewedAnalysis{}, analysisAccessError(ctx)
	}

	return checked, nil
}

func analysisS3Key(key string) bool {
	if key == "" || len(key) > 1024 || strings.ContainsAny(key, "\\") || strings.ContainsFunc(key, unicode.IsControl) {
		return false
	}
	for _, part := range strings.Split(key, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}

	return true
}

func analysisAccessError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	return ErrAnalysisAccess
}
