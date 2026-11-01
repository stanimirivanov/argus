package main

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func analysisEnvironment(t *testing.T) map[string]string {
	t.Helper()
	root := t.TempDir()
	return map[string]string{"ENABLED": "true", "URL": "https://provider.example/integration/v1/requests", "TOKEN": "provider-secret", "PROVIDER": "https://provider.example", "PRINCIPAL_ISSUER": "https://auth.example", "PRINCIPAL_ID": "argus-analysis", "REQUEST_ISSUER": "https://argus.example", "EXPECTED_REVIEWER": "trusted-reviewer", "REVIEW_DIRECTORY": filepath.Join(root, "protected-reviews"), "S3_REGION": "us-east-1", "S3_BUCKET": "perfeng-artifacts", "S3_PREFIX": "reports/", "S3_ACCESS_KEY_ID": "REPORTKEY", "S3_SECRET_ACCESS_KEY": "report-secret"}
}
func loadAnalysisValues(values map[string]string) (performanceAnalysisConfig, error) {
	return loadPerformanceAnalysis(func(name string) string { return values[strings.TrimPrefix(name, "ARGUS_PERFORMANCE_ANALYSIS_")] })
}

func TestAnalysisConfigurationDisabledIsInert(t *testing.T) {
	for _, flag := range []string{"", "false"} {
		c, err := loadPerformanceAnalysis(func(name string) string {
			if name != "ARGUS_PERFORMANCE_ANALYSIS_ENABLED" {
				t.Fatal("disabled analysis read resources", name)
			}
			return flag
		})
		if err != nil || !reflect.DeepEqual(c, performanceAnalysisConfig{}) {
			t.Fatal("disabled analysis activated", err)
		}
	}
}

func TestAnalysisConfigurationKeepsIndependentFrozenCredentials(t *testing.T) {
	values := analysisEnvironment(t)
	values["S3_SESSION_TOKEN"] = "report-session"
	values["S3_ENDPOINT"] = "https://localhost:9443"
	values["S3_PATH_STYLE"] = "true"
	values["CA_FILE"] = filepath.Join(t.TempDir(), "private-ca.pem")
	c, err := loadAnalysisValues(values)
	if err != nil {
		t.Fatal(err)
	}
	if !c.enabled || c.scope.PrincipalID != "argus-analysis" || c.requestIssuer != "https://argus.example" || c.expectedReviewer != "trusted-reviewer" || c.poll != 5*time.Second || c.timeout != 30*time.Second || c.maxPerTick != 1 || !c.reports.UsePathStyle || c.reports.Endpoint != values["S3_ENDPOINT"] || c.caFile != values["CA_FILE"] {
		t.Fatal("trusted policy or defaults changed")
	}
	values["S3_SECRET_ACCESS_KEY"] = "changed"
	credentials, err := c.reports.Credentials.Retrieve(t.Context())
	if err != nil || credentials.AccessKeyID != "REPORTKEY" || credentials.SecretAccessKey != "report-secret" || credentials.SessionToken != "report-session" || c.source.Token != "provider-secret" {
		t.Fatal("credentials were discovered, aliased or crossed")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := c.reports.Credentials.Retrieve(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("credential cancellation lost", err)
	}
}

func TestAnalysisConfigurationRejectsIncompleteOrUnsafeValues(t *testing.T) {
	for _, key := range []string{"URL", "TOKEN", "PROVIDER", "PRINCIPAL_ISSUER", "PRINCIPAL_ID", "REQUEST_ISSUER", "EXPECTED_REVIEWER", "REVIEW_DIRECTORY", "S3_REGION", "S3_BUCKET", "S3_PREFIX", "S3_ACCESS_KEY_ID", "S3_SECRET_ACCESS_KEY"} {
		t.Run("missing-"+key, func(t *testing.T) {
			values := analysisEnvironment(t)
			delete(values, key)
			assertAnalysisConfigurationRejected(t, values)
		})
	}
	for _, change := range [][2]string{
		{"ENABLED", "yes"}, {"URL", "http://remote.example"}, {"REQUEST_ISSUER", "relative"}, {"REQUEST_ISSUER", "https://argus.example#"}, {"REQUEST_ISSUER", "https://user:private@argus.example"}, {"EXPECTED_REVIEWER", "\nprivate"},
		{"REVIEW_DIRECTORY", "relative"}, {"REVIEW_DIRECTORY", "/"}, {"REVIEW_DIRECTORY", "/tmp/../private"}, {"REVIEW_DIRECTORY", "/tmp/private/"}, {"CA_FILE", "~/private"},
		{"S3_REGION", "bad"}, {"S3_PREFIX", "reports/../"}, {"S3_ENDPOINT", "http://localhost:9443"}, {"S3_ENDPOINT", "https://user:private@localhost"}, {"S3_PATH_STYLE", "yes"},
		{"S3_SECRET_ACCESS_KEY", "\nprivate"}, {"S3_SESSION_TOKEN", "\x00private"}, {"S3_ACCESS_KEY_ID", strings.Repeat("x", 4097)},
		{"POLL_INTERVAL", "0s"}, {"POLL_INTERVAL", "1m1s"}, {"TIMEOUT", "1.000000001s"}, {"TIMEOUT", "private"}, {"MAX_PER_TICK", "0"}, {"MAX_PER_TICK", "101"}, {"MAX_PER_TICK", "private"},
	} {
		t.Run(change[0]+"-invalid", func(t *testing.T) {
			values := analysisEnvironment(t)
			values[change[0]] = change[1]
			assertAnalysisConfigurationRejected(t, values)
		})
	}
}

func assertAnalysisConfigurationRejected(t *testing.T, values map[string]string) {
	t.Helper()
	c, err := loadAnalysisValues(values)
	if err == nil || !reflect.DeepEqual(c, performanceAnalysisConfig{}) {
		t.Fatal("partial or invalid configuration accepted")
	}
	for _, private := range []string{"private", "provider-secret", "report-secret", "localhost", "REPORTKEY"} {
		if strings.Contains(err.Error(), private) {
			t.Fatal("private configuration leaked")
		}
	}
}

func TestAnalysisConfigurationDefaultsAndBounds(t *testing.T) {
	values := analysisEnvironment(t)
	c, err := loadAnalysisValues(values)
	if err != nil || c.reports.Endpoint != "" || c.reports.UsePathStyle || c.caFile != "" {
		t.Fatal("AWS defaults changed", err)
	}
	// Preserve exact secret bytes; whitespace is not normalized by the loader.
	values["S3_SECRET_ACCESS_KEY"] = " report-secret "
	values["POLL_INTERVAL"] = "1s"
	values["TIMEOUT"] = "1m"
	values["MAX_PER_TICK"] = "100"
	c, err = loadAnalysisValues(values)
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := c.reports.Credentials.Retrieve(t.Context())
	if err != nil || credentials.SecretAccessKey != " report-secret " || c.poll != time.Second || c.timeout != time.Minute || c.maxPerTick != 100 {
		t.Fatal("explicit bounds or bytes changed", err)
	}
}
