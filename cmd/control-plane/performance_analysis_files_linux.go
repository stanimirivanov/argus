package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/stanimirivanov/argus/internal/performance/adapters/perfeng"
)

var errAnalysisResource = errors.New("protected analysis resource unavailable")

// Open ancestors descriptor-relatively: reject links and untrusted writers. A
// trusted sticky ancestor permits /tmp, but the review root itself must be private.
// Ownership/modes supplement independently trusted issuance, never authenticate it.
func analysisDirectory(path string, private bool) (*os.File, error) {
	fd, err := syscall.Open("/", syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, errAnalysisResource
	}
	current := os.NewFile(uintptr(fd), "analysis directory")
	for _, part := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		if part == "" {
			continue
		}
		next, err := syscall.Openat(int(current.Fd()), part, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
		closeAnalysisFile(current)
		if err != nil {
			return nil, errAnalysisResource
		}
		current = os.NewFile(uintptr(next), "analysis directory")
		if !analysisOwner(current, false, true) {
			closeAnalysisFile(current)
			return nil, errAnalysisResource
		}
	}
	if !analysisOwner(current, private, false) {
		closeAnalysisFile(current)
		return nil, errAnalysisResource
	}

	return current, nil
}

func analysisOwner(file *os.File, private, sticky bool) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || (stat.Uid != 0 && strconv.FormatUint(uint64(stat.Uid), 10) != strconv.Itoa(os.Getuid())) {
		return false
	}
	if !info.IsDir() && stat.Nlink != 1 {
		return false
	}
	mask := os.FileMode(0o022)
	if private {
		mask = 0o077
	}

	return info.Mode().Perm()&mask == 0 || (sticky && info.IsDir() && info.Mode()&os.ModeSticky != 0)
}

func analysisFile(root *os.File, name string, private bool, limit int64) (*os.File, error) {
	if !analysisOwner(root, private, false) {
		return nil, errAnalysisResource
	}
	fd, err := syscall.Openat(int(root.Fd()), name, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, errAnalysisResource
	}
	file := os.NewFile(uintptr(fd), "analysis file")
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit || !analysisOwner(file, private, false) {
		closeAnalysisFile(file)
		return nil, errAnalysisResource
	}

	return file, nil
}

func (c *performanceAnalysisConfig) prepareAnalysis() (*os.File, perfeng.AnalysisConsumer, error) {
	if !c.enabled {
		return nil, perfeng.AnalysisConsumer{}, nil
	}
	root, err := analysisDirectory(c.reviewDirectory, true)
	if err != nil {
		return nil, perfeng.AnalysisConsumer{}, err
	}
	if c.caFile != "" {
		c.reports.Client, err = analysisCAClient(c.caFile)
		if err != nil {
			closeAnalysisFile(root)
			return nil, perfeng.AnalysisConsumer{}, err
		}
	}
	source, err := perfeng.NewAnalysisS3(c.reports)
	if err != nil {
		closeAnalysisFile(root)
		return nil, perfeng.AnalysisConsumer{}, errAnalysisResource
	}

	provider := c.source
	if c.caFile != "" {
		provider.Client = c.reports.Client
	}

	return root, perfeng.AnalysisConsumer{Source: provider, Reports: source, ExpectedReviewer: c.expectedReviewer}, nil
}

func analysisCAClient(path string) (*http.Client, error) {
	root, err := analysisDirectory(filepath.Dir(path), false)
	if err != nil {
		return nil, err
	}
	defer func() { closeAnalysisFile(root) }()
	file, err := analysisFile(root, filepath.Base(path), false, 64<<10)
	if err != nil {
		return nil, err
	}
	defer func() { closeAnalysisFile(file) }()
	raw, err := io.ReadAll(io.LimitReader(file, (64<<10)+1))
	if err != nil || len(raw) > 64<<10 {
		return nil, errAnalysisResource
	}
	roots, err := x509.SystemCertPool()
	if err != nil {
		return nil, errAnalysisResource
	}
	if !roots.AppendCertsFromPEM(raw) {
		return nil, errAnalysisResource
	}
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, errAnalysisResource
	}
	transport := base.Clone()
	transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}

	return &http.Client{Transport: transport}, nil
}

func closeAnalysisFile(file *os.File) {
	_ = file.Close() //nolint:errcheck // Read-only descriptor cleanup cannot repair completed work.
}
