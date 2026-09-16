// Command contractgen regenerates or verifies language bindings for the Argus
// contract kernel. Generator versions are owned by go.mod and uv.lock.
package main

import (
	"bytes"
	"crypto/sha256"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	schemaPath       = "contracts/schemas/kernel/v1/kernel.schema.json"
	goOutputPath     = "contracts/generated/go/kernel/v1/kernel.gen.go"
	pythonOutputPath = "contracts/generated/python/argus_contracts/kernel_v1.py"
)

type generatedFile struct {
	label   string
	target  string
	temp    string
	command *exec.Cmd
}

func main() {
	write := flag.Bool("write", false, "replace checked-in bindings instead of comparing them")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "contractgen does not accept positional arguments")
		os.Exit(2)
	}

	root, err := repositoryRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "contract generation failed: %v\n", err)
		os.Exit(1)
	}
	temporary, err := os.MkdirTemp("", "argus-contractgen-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "contract generation failed: create temporary directory: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if removeErr := os.RemoveAll(temporary); removeErr != nil {
			fmt.Fprintf(os.Stderr, "contract generation warning: remove temporary directory: %v\n", removeErr)
		}
	}()

	files := generationCommands(root, temporary)
	for _, file := range files {
		file.command.Dir = root
		file.command.Stdout = os.Stdout
		file.command.Stderr = os.Stderr
		if err := file.command.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "contract generation failed: generate %s: %v\n", file.label, err)
			os.Exit(1)
		}
		if err := acceptOrCompare(file, *write); err != nil {
			fmt.Fprintf(os.Stderr, "contract generation failed: %v\n", err)
			os.Exit(1)
		}
	}

	if *write {
		fmt.Println("Regenerated Go and Python contract bindings.")
		return
	}
	fmt.Println("Verified Go and Python contract bindings are reproducible.")
}

func repositoryRoot() (string, error) {
	directory, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(directory, "go.mod")); statErr == nil {
			return directory, nil
		} else if !os.IsNotExist(statErr) {
			return "", fmt.Errorf("inspect go.mod: %w", statErr)
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", fmt.Errorf("locate repository root from %s", directory)
		}
		directory = parent
	}
}

func generationCommands(root string, temporary string) []generatedFile {
	goTemp := filepath.Join(temporary, filepath.Base(goOutputPath))
	pythonTemp := filepath.Join(temporary, filepath.Base(pythonOutputPath))

	return []generatedFile{
		{
			label:  "Go binding",
			target: filepath.Join(root, filepath.FromSlash(goOutputPath)),
			temp:   goTemp,
			command: exec.Command(
				"go", "tool", "go-jsonschema",
				"--package", "kernel",
				"--struct-name-from-title",
				"--capitalization", "ID",
				"--capitalization", "URI",
				"--capitalization", "API",
				"--tags", "json",
				"--output", goTemp,
				schemaPath,
			),
		},
		{
			label:  "Python binding",
			target: filepath.Join(root, filepath.FromSlash(pythonOutputPath)),
			temp:   pythonTemp,
			command: exec.Command(
				"uv", "run", "--locked", "datamodel-codegen",
				"--input", schemaPath,
				"--input-file-type", "jsonschema",
				"--output-model-type", "pydantic_v2.BaseModel",
				"--preset", "practical-py312-20260619",
				"--use-title-as-name",
				"--extra-fields", "forbid",
				"--formatters", "ruff-check", "ruff-format",
				"--output", pythonTemp,
			),
		},
	}
}

func acceptOrCompare(file generatedFile, write bool) error {
	generated, err := os.ReadFile(file.temp)
	if err != nil {
		return fmt.Errorf("read generated %s: %w", file.label, err)
	}
	if write {
		if err := os.WriteFile(file.target, generated, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", file.target, err)
		}
		return nil
	}

	committed, err := os.ReadFile(file.target)
	if err != nil {
		return fmt.Errorf("read checked-in %s: %w", file.label, err)
	}
	if bytes.Equal(generated, committed) {
		return nil
	}

	return fmt.Errorf(
		"%s is stale (generated sha256 %x, checked-in sha256 %x); run make generate-contracts",
		file.target,
		sha256.Sum256(generated),
		sha256.Sum256(committed),
	)
}
