package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const outputLimit = 4 << 20

var verificationTargets = map[string]time.Duration{
	"build":              5 * time.Minute,
	"fmt-check":          5 * time.Minute,
	"docs-check":         5 * time.Minute,
	"architecture-check": 5 * time.Minute,
	"check":              15 * time.Minute,
	"test":               15 * time.Minute,
	"verify":             20 * time.Minute,
	"race":               20 * time.Minute,
}

type checkReport struct {
	SchemaVersion      int       `json:"schemaVersion"`
	TaskID             string    `json:"taskId"`
	TaskSHA256         string    `json:"taskSha256"`
	HostStateSHA256    string    `json:"hostStateSha256"`
	Profile            string    `json:"profile"`
	Repository         string    `json:"repository"`
	Check              string    `json:"check"`
	Command            []string  `json:"command"`
	CommandSHA256      string    `json:"commandSha256,omitempty"`
	StartedAt          time.Time `json:"startedAt"`
	FinishedAt         time.Time `json:"finishedAt"`
	DurationMillis     int64     `json:"durationMillis"`
	ExitCode           int       `json:"exitCode"`
	TimedOut           bool      `json:"timedOut"`
	LaunchError        string    `json:"launchError,omitempty"`
	Stdout             string    `json:"stdout"`
	Stderr             string    `json:"stderr"`
	StdoutBytes        int64     `json:"stdoutBytes"`
	StderrBytes        int64     `json:"stderrBytes"`
	StdoutTruncated    bool      `json:"stdoutTruncated"`
	StderrTruncated    bool      `json:"stderrTruncated"`
	EnvironmentLimited bool      `json:"environmentLimited"`
}

func runVerification(ctx context.Context, p policy, check string) (checkReport, error) {
	if p.task.Profile != "implementation" {
		return checkReport{}, errors.New("verification execution is unavailable in the audit profile")
	}
	timeout, ok := verificationTargets[check]
	if !ok || !slices.Contains(p.task.Checks, check) {
		return checkReport{}, fmt.Errorf("check %q is not allowed by this task", check)
	}
	if _, err := os.Stat(filepath.Join(p.repository, "Makefile")); err != nil {
		return checkReport{}, fmt.Errorf("repository Makefile: %w", err)
	}

	started := time.Now().UTC()
	report := checkReport{
		SchemaVersion:      1,
		TaskID:             p.task.TaskID,
		TaskSHA256:         p.taskDigest,
		HostStateSHA256:    p.hostStateDigest,
		Profile:            p.task.Profile,
		Repository:         p.repository,
		Check:              check,
		Command:            []string{"make", check},
		StartedAt:          started,
		ExitCode:           -1,
		EnvironmentLimited: true,
	}
	makePath, err := exec.LookPath("make")
	if err == nil {
		makePath, err = filepath.Abs(makePath)
	}
	if err == nil {
		makePath, err = filepath.EvalSymlinks(makePath)
	}
	if err == nil && (within(p.repository, makePath) || within(p.scratchRoot, makePath)) {
		err = errors.New("make executable must be outside repository and scratch root")
	}
	if err != nil {
		report.LaunchError = err.Error()
		report.FinishedAt = time.Now().UTC()
		report.DurationMillis = report.FinishedAt.Sub(started).Milliseconds()
		return report, nil
	}
	report.Command[0] = makePath
	report.CommandSHA256, err = fileDigest(makePath)
	if err != nil {
		report.LaunchError = fmt.Sprintf("hash make executable: %v", err)
		report.FinishedAt = time.Now().UTC()
		report.DurationMillis = report.FinishedAt.Sub(started).Milliseconds()
		return report, nil
	}
	moduleCache, err := resolveModuleCache()
	if err != nil {
		report.LaunchError = fmt.Sprintf("resolve read-only Go module cache: %v", err)
		report.FinishedAt = time.Now().UTC()
		report.DurationMillis = report.FinishedAt.Sub(started).Milliseconds()
		return report, nil
	}

	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result := executeProcess(commandCtx, makePath, []string{check}, p.repository, limitedEnvironment(p.scratchRoot, moduleCache))
	report.FinishedAt = result.finishedAt
	report.DurationMillis = report.FinishedAt.Sub(started).Milliseconds()
	report.Stdout = result.stdout.String()
	report.Stderr = result.stderr.String()
	report.StdoutBytes = result.stdout.total
	report.StderrBytes = result.stderr.total
	report.StdoutTruncated = result.stdout.total > outputLimit
	report.StderrTruncated = result.stderr.total > outputLimit
	report.TimedOut = errors.Is(commandCtx.Err(), context.DeadlineExceeded)
	report.ExitCode = result.exitCode
	report.LaunchError = result.launchError

	return report, nil
}

type processResult struct {
	finishedAt  time.Time
	exitCode    int
	launchError string
	stdout      boundedCapture
	stderr      boundedCapture
}

func executeProcess(ctx context.Context, path string, args []string, directory string, environment []string) processResult {
	process := exec.CommandContext(ctx, path, args...)
	process.Dir = directory
	process.Env = environment
	stdout := &boundedCapture{limit: outputLimit}
	stderr := &boundedCapture{limit: outputLimit}
	process.Stdout = stdout
	process.Stderr = stderr
	runErr := process.Run()
	result := processResult{finishedAt: time.Now().UTC(), exitCode: -1, stdout: *stdout, stderr: *stderr}
	if runErr == nil {
		result.exitCode = 0
	} else {
		var exitError *exec.ExitError
		if errors.As(runErr, &exitError) {
			result.exitCode = exitError.ExitCode()
		} else {
			result.launchError = runErr.Error()
		}
	}

	return result
}

func limitedEnvironment(scratchRoot, moduleCache string) []string {
	// No arbitrary inherited variables: this excludes token and cloud-credential
	// variables. Filesystem and socket isolation still belong to the host.
	allowed := []string{"PATH", "PATHEXT", "SystemRoot", "WINDIR", "COMSPEC"}
	environment := make([]string, 0, len(allowed)+13)
	for _, name := range allowed {
		if value, present := os.LookupEnv(name); present {
			environment = append(environment, name+"="+value)
		}
	}
	for _, name := range []string{"HOME", "USERPROFILE", "TMP", "TEMP", "TMPDIR", "XDG_CACHE_HOME", "GOCACHE", "GOPATH"} {
		environment = append(environment, name+"="+scratchRoot)
	}
	environment = append(environment,
		"GOMODCACHE="+moduleCache,
		"CI=1",
		"GOTOOLCHAIN=local",
		"GOPROXY=off",
		"GOSUMDB=off",
		"npm_config_offline=true",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_NOSYSTEM=1",
	)

	return environment
}

func resolveModuleCache() (string, error) {
	moduleCache := os.Getenv("GOMODCACHE")
	if moduleCache == "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, "go", "env", "GOMODCACHE")
		command.Env = append(os.Environ(), "GOTOOLCHAIN=local", "GOPROXY=off")
		output, err := command.Output()
		if err != nil {
			return "", fmt.Errorf("go env GOMODCACHE: %w", err)
		}
		moduleCache = strings.TrimSpace(string(output))
	}
	if !filepath.IsAbs(moduleCache) {
		return "", fmt.Errorf("go module cache %q is not absolute", moduleCache)
	}
	// This path is a read-only input granted by the host. Do not require this
	// process to resolve it: a correctly confined child may not have metadata
	// access to the cache until the host mounts its read-only view.
	return filepath.Clean(moduleCache), nil
}

type boundedCapture struct {
	buffer bytes.Buffer
	total  int64
	limit  int64
}

func (capture *boundedCapture) Write(data []byte) (int, error) {
	capture.total += int64(len(data))
	if int64(len(data)) >= capture.limit {
		capture.buffer.Reset()
		if _, err := capture.buffer.Write(data[len(data)-int(capture.limit):]); err != nil {
			return 0, err
		}
	} else {
		overflow := int64(capture.buffer.Len()+len(data)) - capture.limit
		if overflow > 0 {
			capture.buffer.Next(int(overflow))
		}
		if _, err := capture.buffer.Write(data); err != nil {
			return 0, err
		}
	}

	return len(data), nil
}

func (capture *boundedCapture) String() string {
	return capture.buffer.String()
}

func writeReport(scratchRoot string, report checkReport) (result string, err error) {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	file, err := os.CreateTemp(scratchRoot, report.TaskID+"-"+report.Check+"-*.tmp")
	if err != nil {
		return "", err
	}
	temporary := file.Name()
	defer func() {
		removeErr := os.Remove(temporary)
		if removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			err = errors.Join(err, fmt.Errorf("remove temporary report: %w", removeErr))
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		return "", errors.Join(err, file.Close())
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		return "", errors.Join(err, file.Close())
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	finalPath := temporary[:len(temporary)-len(".tmp")] + ".json"
	if err := os.Rename(temporary, finalPath); err != nil {
		return "", err
	}

	return finalPath, nil
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func fileDigest(path string) (value string, err error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}
