package main

import (
	"context"
	"errors"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stanimirivanov/argus/internal/performance/adapters/perfeng"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

// performanceAnalysisConfig holds host-issued policy only. Parsing starts no work
// and does not authenticate paths, reviewers or IAM grants. Activation must verify
// protected file origin/access and load CA roots before constructing the consumer.
// Never log this value: source and report configuration contain credentials.
type performanceAnalysisConfig struct {
	enabled                                                  bool
	source                                                   perfeng.IntegrationHTTP
	scope                                                    requestintent.Scope
	requestIssuer, expectedReviewer, reviewDirectory, caFile string
	reports                                                  perfeng.AnalysisS3Config
	poll, timeout                                            time.Duration
	maxPerTick                                               int
}

// loadPerformanceAnalysis is a pure configuration boundary, independent of request
// dispatch enablement. Disabled mode reads only ENABLED; enabled failures return
// zero configuration and constant diagnostics without echoing host input. Resource
// paths and secrets are not trimmed, expanded, opened or discovered implicitly.
func loadPerformanceAnalysis(getenv func(string) string) (performanceAnalysisConfig, error) {
	read := func(suffix string) string { return getenv("ARGUS_PERFORMANCE_ANALYSIS_" + suffix) }
	enabled := strings.TrimSpace(read("ENABLED"))
	if enabled == "" || enabled == "false" {
		return performanceAnalysisConfig{}, nil
	}
	if enabled != "true" {
		return performanceAnalysisConfig{}, errors.New("ARGUS_PERFORMANCE_ANALYSIS_ENABLED must be true or false")
	}
	c := performanceAnalysisConfig{enabled: true}
	c.source = perfeng.IntegrationHTTP{Endpoint: read("URL"), Token: read("TOKEN"), Provider: read("PROVIDER"), Principal: perfeng.IntegrationIdentity{Issuer: read("PRINCIPAL_ISSUER"), ID: read("PRINCIPAL_ID")}}
	c.scope = requestintent.Scope{Issuer: c.source.Provider, PrincipalIssuer: c.source.Principal.Issuer, PrincipalID: c.source.Principal.ID}
	c.requestIssuer = read("REQUEST_ISSUER")
	issuer, err := url.Parse(c.requestIssuer)
	if err != nil || !issuer.IsAbs() || issuer.User != nil || issuer.RawQuery != "" || issuer.ForceQuery || strings.Contains(c.requestIssuer, "#") || !requestintent.ValidRequestIssuer(c.requestIssuer) || c.source.ValidateDispatch(c.scope) != nil {
		return performanceAnalysisConfig{}, errors.New("invalid analysis provider or scope configuration")
	}
	c.expectedReviewer, c.reviewDirectory, c.caFile = read("EXPECTED_REVIEWER"), read("REVIEW_DIRECTORY"), read("CA_FILE")
	if !analysisConfigText(c.expectedReviewer, 256) || !analysisConfigPath(c.reviewDirectory) || (c.caFile != "" && !analysisConfigPath(c.caFile)) {
		return performanceAnalysisConfig{}, errors.New("invalid analysis reviewer or protected resource path")
	}
	c.reports, err = analysisReportConfiguration(read)
	if err != nil {
		return performanceAnalysisConfig{}, err
	}
	c.poll, err = analysisConfigDuration(read("POLL_INTERVAL"), 5*time.Second)
	if err != nil {
		return performanceAnalysisConfig{}, err
	}
	c.timeout, err = analysisConfigDuration(read("TIMEOUT"), 30*time.Second)
	if err != nil {
		return performanceAnalysisConfig{}, err
	}
	c.maxPerTick = 1
	if raw := read("MAX_PER_TICK"); raw != "" {
		c.maxPerTick, err = strconv.Atoi(raw)
		if err != nil || c.maxPerTick < 1 || c.maxPerTick > 100 {
			return performanceAnalysisConfig{}, errors.New("analysis MAX_PER_TICK must be 1..100")
		}
	}

	return c, nil
}

func analysisReportConfiguration(read func(string) string) (perfeng.AnalysisS3Config, error) {
	credentials := aws.Credentials{AccessKeyID: read("S3_ACCESS_KEY_ID"), SecretAccessKey: read("S3_SECRET_ACCESS_KEY"), SessionToken: read("S3_SESSION_TOKEN")}
	if !analysisConfigText(credentials.AccessKeyID, 4096) || !analysisConfigText(credentials.SecretAccessKey, 4096) || (credentials.SessionToken != "" && !analysisConfigText(credentials.SessionToken, 4096)) {
		return perfeng.AnalysisS3Config{}, errors.New("explicit analysis report credentials are required")
	}
	config := perfeng.AnalysisS3Config{Region: read("S3_REGION"), Bucket: read("S3_BUCKET"), Prefix: read("S3_PREFIX"), Endpoint: read("S3_ENDPOINT"), Credentials: aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
		if err := ctx.Err(); err != nil {
			return aws.Credentials{}, err
		}
		return credentials, nil
	})}
	switch strings.TrimSpace(read("S3_PATH_STYLE")) {
	case "", "false":
	case "true":
		config.UsePathStyle = true
	default:
		return perfeng.AnalysisS3Config{}, errors.New("analysis S3_PATH_STYLE must be true or false")
	}
	// Constructor validation does not retrieve credentials or perform I/O. A later
	// activation step must install the selected CA trust before using this config.
	if _, err := perfeng.NewAnalysisS3(config); err != nil {
		return perfeng.AnalysisS3Config{}, errors.New("invalid analysis report access configuration")
	}

	return config, nil
}

func analysisConfigText(value string, maximum int) bool {
	return len(value) <= maximum && strings.TrimSpace(value) != "" && !strings.ContainsFunc(value, unicode.IsControl)
}

func analysisConfigPath(value string) bool {
	return analysisConfigText(value, 4096) && filepath.IsAbs(value) && filepath.Clean(value) == value && filepath.Dir(value) != value
}

func analysisConfigDuration(raw string, fallback time.Duration) (time.Duration, error) {
	if raw == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(raw)
	if err != nil || duration < time.Second || duration > time.Minute || duration%time.Microsecond != 0 {
		return 0, errors.New("analysis intervals must be 1s..1m in whole microseconds")
	}

	return duration, nil
}
