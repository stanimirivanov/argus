// Command agent-harness validates task-scoped agent policy and records checks.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

const policyVersion = 1

var taskIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,79}$`)

// taskPolicy is supplied by a trusted caller outside the checked-out repository.
// Writable paths are repository-relative and use forward slashes on every OS.
type taskPolicy struct {
	Version       int      `json:"version"`
	TaskID        string   `json:"taskId"`
	Profile       string   `json:"profile"`
	Repository    string   `json:"repository"`
	WritableRoots []string `json:"writableRoots"`
	Network       string   `json:"network"`
	Credentials   string   `json:"credentials"`
	Checks        []string `json:"checks"`
}

// hostState is a consistency declaration. It cannot attest that the host
// actually applied filesystem, network, or credential isolation.
type hostState struct {
	Version       int      `json:"version"`
	Repository    string   `json:"repository"`
	WritableRoots []string `json:"writableRoots"`
	ScratchRoot   string   `json:"scratchRoot"`
	Network       string   `json:"network"`
	Credentials   string   `json:"credentials"`
}

type policy struct {
	task            taskPolicy
	host            hostState
	repository      string
	scratchRoot     string
	writableRoots   []string
	taskDigest      string
	hostStateDigest string
}

func loadPolicy(repository, taskFile, hostFile string) (policy, error) {
	repository, err := existingDirectory(repository)
	if err != nil {
		return policy{}, fmt.Errorf("repository: %w", err)
	}
	if taskFile == "" || hostFile == "" {
		return policy{}, errors.New("task and host-state files are required")
	}
	if err := externalPolicyFile(repository, "task", taskFile); err != nil {
		return policy{}, err
	}
	if err := externalPolicyFile(repository, "host state", hostFile); err != nil {
		return policy{}, err
	}

	var task taskPolicy
	taskBytes, err := decodeFile(taskFile, &task)
	if err != nil {
		return policy{}, fmt.Errorf("task: %w", err)
	}
	var host hostState
	hostBytes, err := decodeFile(hostFile, &host)
	if err != nil {
		return policy{}, fmt.Errorf("host state: %w", err)
	}
	if err := validateTaskAndHost(repository, task, host); err != nil {
		return policy{}, err
	}
	roots, err := validateWritableRoots(repository, task.WritableRoots, host.WritableRoots)
	if err != nil {
		return policy{}, err
	}
	if err := validateChecks(task.Checks); err != nil {
		return policy{}, err
	}
	scratchRoot, err := validateScratch(repository, host.ScratchRoot)
	if err != nil {
		return policy{}, err
	}

	return policy{
		task:            task,
		host:            host,
		repository:      repository,
		scratchRoot:     scratchRoot,
		writableRoots:   roots,
		taskDigest:      digest(taskBytes),
		hostStateDigest: digest(hostBytes),
	}, nil
}

func externalPolicyFile(repository, name, path string) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("%s path: %w", name, err)
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("%s path: %w", name, err)
	}
	if within(repository, path) {
		return fmt.Errorf("%s must be supplied from outside the repository", name)
	}

	return nil
}

func validateTaskAndHost(repository string, task taskPolicy, host hostState) error {
	if task.Version != policyVersion || host.Version != policyVersion {
		return fmt.Errorf("unsupported policy version: task=%d host=%d", task.Version, host.Version)
	}
	if !taskIDPattern.MatchString(task.TaskID) {
		return errors.New("taskId must be 1-80 safe filename characters")
	}
	if task.Profile != "audit" && task.Profile != "implementation" {
		return errors.New("profile must be audit or implementation")
	}
	if task.Network != "deny" || host.Network != "deny" {
		return errors.New("network must be denied by both task and host")
	}
	if task.Credentials != "none" || host.Credentials != "none" {
		return errors.New("credentials must be absent in both task and host")
	}
	if !sameDirectory(repository, task.Repository) || !sameDirectory(repository, host.Repository) {
		return errors.New("task and host repository must match --repo")
	}

	if task.Profile == "audit" && (len(task.WritableRoots) != 0 || len(task.Checks) != 0) {
		return errors.New("audit profile cannot write repository paths or run checks")
	}
	if task.Profile == "implementation" && (len(task.WritableRoots) == 0 || len(task.Checks) == 0) {
		return errors.New("implementation profile requires scoped writable roots and checks")
	}

	return nil
}

func validateWritableRoots(repository string, taskRoots, declaredHostRoots []string) ([]string, error) {
	roots := make([]string, 0, len(taskRoots))
	for _, relative := range taskRoots {
		root, err := resolveWritableRoot(repository, relative)
		if err != nil {
			return nil, err
		}
		if slices.Contains(roots, root) {
			return nil, fmt.Errorf("duplicate writable root %q", relative)
		}
		roots = append(roots, root)
	}
	slices.Sort(roots)
	if len(declaredHostRoots) != len(roots) {
		return nil, errors.New("host writable roots do not match task scope")
	}
	hostRoots := make([]string, 0, len(declaredHostRoots))
	for _, root := range declaredHostRoots {
		if !filepath.IsAbs(root) {
			return nil, errors.New("host writable roots must be absolute")
		}
		canonical, err := canonicalExistingPrefix(root)
		if err != nil {
			return nil, fmt.Errorf("host writable root: %w", err)
		}
		if !within(repository, canonical) {
			return nil, errors.New("host writable root escapes repository")
		}
		hostRoots = append(hostRoots, canonical)
	}
	slices.Sort(hostRoots)
	if !slices.Equal(roots, hostRoots) {
		return nil, errors.New("host writable roots do not match task scope")
	}

	return roots, nil
}

func validateChecks(checks []string) error {
	seenChecks := make(map[string]bool)
	for _, check := range checks {
		if _, ok := verificationTargets[check]; !ok || seenChecks[check] {
			return fmt.Errorf("unknown or duplicate verification check %q", check)
		}
		seenChecks[check] = true
	}

	return nil
}

func validateScratch(repository, scratch string) (string, error) {
	if !filepath.IsAbs(scratch) {
		return "", errors.New("host scratchRoot must be absolute")
	}
	scratchRoot, err := existingDirectory(scratch)
	if err != nil {
		return "", fmt.Errorf("host scratchRoot: %w", err)
	}
	if within(repository, scratchRoot) || within(scratchRoot, repository) {
		return "", errors.New("host scratchRoot must be separate from the repository")
	}

	return scratchRoot, nil
}

func decodeFile(path string, into any) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) > 64*1024 {
		return nil, errors.New("policy file exceeds 64 KiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, errors.New("policy file must contain one JSON object")
	}

	return data, nil
}

func existingDirectory(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%q is not a directory", path)
	}

	return path, nil
}

func sameDirectory(expected, actual string) bool {
	if !filepath.IsAbs(actual) {
		return false
	}
	actual, err := existingDirectory(actual)

	return err == nil && filepath.Clean(expected) == filepath.Clean(actual)
}

func within(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func resolveWritableRoot(repository, relative string) (string, error) {
	if relative == "" || relative == "." || strings.Contains(relative, `\`) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("writable root %q must be a scoped repository-relative slash path", relative)
	}
	parts := strings.Split(relative, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.ContainsAny(part, ":\x00") {
			return "", fmt.Errorf("writable root %q contains an invalid component", relative)
		}
	}
	if parts[0] == ".git" {
		return "", errors.New("writable roots cannot include .git")
	}
	root, err := canonicalExistingPrefix(filepath.Join(append([]string{repository}, parts...)...))
	if err != nil {
		return "", fmt.Errorf("writable root %q: %w", relative, err)
	}
	if !within(repository, root) || root == repository {
		return "", fmt.Errorf("writable root %q escapes repository or grants the whole repository", relative)
	}

	return root, nil
}

// canonicalExistingPrefix resolves every existing path component. This rejects
// a symlink escape even when the final writable path has not been created yet.
func canonicalExistingPrefix(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	missing := make([]string, 0)
	current := path
	for {
		_, err := os.Lstat(current)
		if err == nil {
			current, err = filepath.EvalSymlinks(current)
			if err != nil {
				return "", err
			}

			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", errors.New("path has no existing ancestor")
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
	for i := len(missing) - 1; i >= 0; i-- {
		current = filepath.Join(current, missing[i])
	}

	return filepath.Clean(current), nil
}
