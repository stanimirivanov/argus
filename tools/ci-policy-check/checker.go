// Package main checks the checked-in validation workflow against CI policy.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

const osExpression = "${{ matrix.os }}"

type ciPolicy struct {
	Jobs map[string]jobPolicy `json:"jobs"`
}

type jobPolicy struct {
	Name       string   `json:"name"`
	RunsOn     string   `json:"runs_on"`
	MatrixOS   []string `json:"matrix_os"`
	MakeTarget string   `json:"make_target"`
}

type validationWorkflow struct {
	On       map[string]yaml.Node `yaml:"on"`
	Env      map[string]yaml.Node `yaml:"env"`
	Defaults struct {
		Run yaml.Node `yaml:"run"`
	} `yaml:"defaults"`
	Jobs map[string]validationJob `yaml:"jobs"`
}

type validationJob struct {
	Name            string               `yaml:"name"`
	RunsOn          string               `yaml:"runs-on"`
	If              yaml.Node            `yaml:"if"`
	Needs           yaml.Node            `yaml:"needs"`
	ContinueOnError yaml.Node            `yaml:"continue-on-error"`
	Env             map[string]yaml.Node `yaml:"env"`
	Defaults        struct {
		Run yaml.Node `yaml:"run"`
	} `yaml:"defaults"`
	Steps    []validationStep `yaml:"steps"`
	Strategy struct {
		Matrix yaml.Node `yaml:"matrix"`
	} `yaml:"strategy"`
}

type validationStep struct {
	Run              string               `yaml:"run"`
	If               yaml.Node            `yaml:"if"`
	ContinueOnError  yaml.Node            `yaml:"continue-on-error"`
	Env              map[string]yaml.Node `yaml:"env"`
	Shell            yaml.Node            `yaml:"shell"`
	WorkingDirectory yaml.Node            `yaml:"working-directory"`
}

func loadPolicy(data []byte) (ciPolicy, error) {
	var policy ciPolicy
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&policy); err != nil {
		return ciPolicy{}, fmt.Errorf("decode policy: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return ciPolicy{}, fmt.Errorf("policy contains trailing data")
	}
	if err := validatePolicy(policy); err != nil {
		return ciPolicy{}, err
	}

	return policy, nil
}

func validatePolicy(policy ciPolicy) error {
	if len(policy.Jobs) == 0 {
		return fmt.Errorf("policy has no jobs")
	}
	contexts := make(map[string]string)
	for _, id := range sortedJobIDs(policy.Jobs) {
		job := policy.Jobs[id]
		if id == "" || job.Name == "" || job.RunsOn == "" || job.MakeTarget == "" {
			return fmt.Errorf("job %q has a missing policy field", id)
		}
		if err := validateJobPolicy(id, job, contexts); err != nil {
			return err
		}
	}

	return nil
}

func validateJobPolicy(id string, job jobPolicy, contexts map[string]string) error {
	nameWithoutOS := strings.ReplaceAll(job.Name, osExpression, "")
	if strings.Contains(nameWithoutOS, "${{") || strings.Contains(job.RunsOn, "${{") && job.RunsOn != osExpression {
		return fmt.Errorf("job %q has an unsupported dynamic name or runner", id)
	}
	if len(job.MatrixOS) == 0 && strings.Contains(job.Name, osExpression) {
		return fmt.Errorf("job %q names matrix.os without an OS matrix", id)
	}
	if len(job.MatrixOS) != 0 && (!strings.Contains(job.Name, osExpression) || job.RunsOn != osExpression) {
		return fmt.Errorf("job %q must name and run on matrix.os", id)
	}
	for _, osName := range job.MatrixOS {
		if osName == "" || strings.Contains(osName, "${{") {
			return fmt.Errorf("job %q has an invalid OS matrix value", id)
		}
	}
	if len(job.MatrixOS) == 0 {
		return addContext(contexts, id, job.Name)
	}
	for _, osName := range job.MatrixOS {
		if err := addContext(contexts, id, strings.ReplaceAll(job.Name, osExpression, osName)); err != nil {
			return err
		}
	}

	return nil
}

func addContext(contexts map[string]string, id, name string) error {
	if prior, exists := contexts[name]; exists {
		return fmt.Errorf("status name %q is duplicated by jobs %q and %q", name, prior, id)
	}
	contexts[name] = id

	return nil
}

func checkWorkflow(policy ciPolicy, data []byte) ([]string, error) {
	var workflow validationWorkflow
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		return nil, err
	}
	var drift []string
	if !hasUnfilteredPullRequest(workflow.On) {
		drift = append(drift, "workflow must have an unfiltered pull_request trigger")
	}
	drift = append(drift, forbiddenMakeEnv("workflow", workflow.Env)...)
	if workflow.Defaults.Run.Kind != 0 {
		drift = append(drift, "workflow must not override defaults.run")
	}
	for _, id := range sortedJobIDs(policy.Jobs) {
		want := policy.Jobs[id]
		got, exists := workflow.Jobs[id]
		if !exists {
			drift = append(drift, fmt.Sprintf("missing job %q", id))
			continue
		}
		drift = append(drift, checkJob(id, want, got)...)
	}
	for id := range workflow.Jobs {
		if _, expected := policy.Jobs[id]; !expected {
			drift = append(drift, fmt.Sprintf("unexpected job %q", id))
		}
	}
	sort.Strings(drift)

	return drift, nil
}

func hasUnfilteredPullRequest(events map[string]yaml.Node) bool {
	pullRequest, exists := events["pull_request"]
	if !exists {
		return false
	}

	return pullRequest.Tag == "!!null" || pullRequest.Kind == yaml.MappingNode && len(pullRequest.Content) == 0
}

func checkJob(id string, want jobPolicy, got validationJob) []string {
	var drift []string
	if got.Name != want.Name {
		drift = append(drift, fmt.Sprintf("job %q display name: want %q, got %q", id, want.Name, got.Name))
	}
	if got.RunsOn != want.RunsOn {
		drift = append(drift, fmt.Sprintf("job %q runs-on: want %q, got %q", id, want.RunsOn, got.RunsOn))
	}
	if got.If.Kind != 0 || got.Needs.Kind != 0 || got.ContinueOnError.Kind != 0 {
		drift = append(drift, fmt.Sprintf("job %q must not use if, needs, or continue-on-error", id))
	}
	drift = append(drift, forbiddenMakeEnv(fmt.Sprintf("job %q", id), got.Env)...)
	if got.Defaults.Run.Kind != 0 {
		drift = append(drift, fmt.Sprintf("job %q must not override defaults.run", id))
	}
	drift = append(drift, checkMatrix(id, want.MatrixOS, got.Strategy.Matrix)...)
	drift = append(drift, checkMakeStep(id, want.MakeTarget, got.Steps)...)

	return drift
}

func checkMatrix(id string, want []string, matrix yaml.Node) []string {
	if len(want) == 0 {
		if matrix.Kind != 0 {
			return []string{fmt.Sprintf("job %q has an unexpected matrix", id)}
		}
		return nil
	}
	if matrix.Kind != yaml.MappingNode {
		return []string{fmt.Sprintf("job %q must have a fixed OS matrix", id)}
	}
	var drift []string
	var actualOS []string
	seenOS := false
	for index := 0; index < len(matrix.Content); index += 2 {
		axis := matrix.Content[index].Value
		if axis != "os" {
			drift = append(drift, fmt.Sprintf("job %q has unexpected matrix axis %q", id, axis))
			continue
		}
		seenOS = true
		osValues := matrix.Content[index+1]
		if osValues.Kind != yaml.SequenceNode || osValues.Decode(&actualOS) != nil {
			drift = append(drift, fmt.Sprintf("job %q matrix.os must be a fixed list", id))
		}
	}
	if !seenOS || !slices.Equal(actualOS, want) {
		drift = append(drift, fmt.Sprintf("job %q matrix.os: want %q, got %q", id, want, actualOS))
	}

	return drift
}

func checkMakeStep(id, target string, steps []validationStep) []string {
	command := "make " + target
	var drift []string
	found := false
	for _, step := range steps {
		if strings.TrimSpace(step.Run) != command {
			continue
		}
		found = true
		if step.If.Kind != 0 || step.ContinueOnError.Kind != 0 {
			drift = append(drift, fmt.Sprintf("job %q must run %q without if or continue-on-error", id, command))
		}
		if step.Shell.Kind != 0 || step.WorkingDirectory.Kind != 0 {
			drift = append(drift, fmt.Sprintf("job %q must run %q without shell or working-directory override", id, command))
		}
		drift = append(drift, forbiddenMakeEnv(fmt.Sprintf("job %q required Make step", id), step.Env)...)
	}
	if !found {
		drift = append(drift, fmt.Sprintf("job %q must run %q", id, command))
	}

	return drift
}

func forbiddenMakeEnv(scope string, env map[string]yaml.Node) []string {
	var drift []string
	for key := range env {
		switch strings.ToUpper(key) {
		case "MAKEFLAGS", "MFLAGS", "GNUMAKEFLAGS", "MAKEFILES":
			drift = append(drift, fmt.Sprintf("%s must not set %s", scope, key))
		}
	}

	return drift
}

func sortedJobIDs(jobs map[string]jobPolicy) []string {
	ids := make([]string, 0, len(jobs))
	for id := range jobs {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	return ids
}
