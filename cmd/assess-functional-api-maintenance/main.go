// Command assess-functional-api-maintenance classifies immutable API test evidence.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/stanimirivanov/argus/internal/adaptation/functionalapi"
	"github.com/stanimirivanov/argus/internal/change"
	changecontract "github.com/stanimirivanov/argus/internal/change/adapters/contract"
	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/execution"
	executioncontract "github.com/stanimirivanov/argus/internal/execution/adapters/contract"
	"github.com/stanimirivanov/argus/internal/selection"
	selectioncontract "github.com/stanimirivanov/argus/internal/selection/adapters/contract"
)

var commandSpec = commandline.Spec{
	Name:     "assess-functional-api-maintenance",
	Synopsis: "assess-functional-api-maintenance -impact FILE -manifest FILE -inventory FILE -attempt-directory DIR",
	Role:     "CI worker",
}

func main() {
	if code, handled := commandline.HandleMeta(os.Args[1:], os.Stdout, os.Stderr, commandSpec); handled {
		os.Exit(code)
	}
	impactPath := flag.String("impact", "", "capability impact JSON")
	manifestPath := flag.String("manifest", "", "reviewed execution manifest JSON")
	inventoryPath := flag.String("inventory", "", "complete head test inventory JSON")
	attemptDirectory := flag.String("attempt-directory", "", "directory of execution attempts")
	flag.Parse()
	if *impactPath == "" || *manifestPath == "" || *inventoryPath == "" || *attemptDirectory == "" || flag.NArg() != 0 {
		fail(fmt.Errorf("usage: assess-functional-api-maintenance -impact FILE -manifest FILE -inventory FILE -attempt-directory DIR"))
	}
	impact := readImpact(*impactPath)
	manifestData := read(*manifestPath)
	manifest, err := selectioncontract.ImportExecutableManifest(manifestData)
	if err != nil {
		fail(err)
	}
	expected := readInventory(*inventoryPath, impact, manifest)
	observations := readObservations(*attemptDirectory, manifest, expected)
	report, err := functionalapi.AssessMaintenance(impact, manifest, observations)
	if err != nil {
		fail(err)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		fail(err)
	}
	validated, err := contracts.DecodeMaintenanceAssessmentV1(encoded)
	if err != nil {
		fail(err)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(validated); err != nil {
		fail(err)
	}
}

func readImpact(path string) change.CapabilityImpact {
	data := read(path)
	var envelope struct {
		APIVersion string `json:"apiVersion"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		fail(err)
	}
	var impact change.CapabilityImpact
	var err error
	if envelope.APIVersion == change.CompositeImpactAPIVersion {
		document, decodeErr := contracts.DecodeCapabilityImpactV2(data)
		if decodeErr != nil {
			fail(decodeErr)
		}
		impact, err = changecontract.ImportCapabilityImpactV2(document)
	} else {
		document, decodeErr := contracts.DecodeCapabilityImpactV1(data)
		if decodeErr != nil {
			fail(decodeErr)
		}
		impact, err = changecontract.ImportCapabilityImpactV1(document)
	}
	if err != nil {
		fail(err)
	}

	return impact
}

func readInventory(path string, impact change.CapabilityImpact, manifest selection.Manifest) map[string]contracts.HeadInventoryTest {
	data := read(path)
	inventory, err := contracts.DecodeHeadTestInventoryV1(data)
	if err != nil {
		fail(err)
	}
	if !inventory.Complete || manifest.InventoryDigest != selection.DigestBytes(data) ||
		inventory.SourceRepository.ProviderRepositoryID != impact.Change.SourceRepository.Identity.ProviderRepositoryID {
		fail(fmt.Errorf("head inventory provenance differs from the reviewed manifest"))
	}
	expected := make(map[string]contracts.HeadInventoryTest, len(inventory.Tests))
	for _, item := range inventory.Tests {
		key := item.TestRepository.ProviderRepositoryID + "\x00" + item.SuiteKey + "\x00" + item.TestKey
		if _, exists := expected[key]; exists {
			fail(fmt.Errorf("duplicate head inventory test"))
		}
		expected[key] = item
	}

	return expected
}

func readObservations(directory string, manifest selection.Manifest, expected map[string]contracts.HeadInventoryTest) []functionalapi.Observation {
	manifestSHA, err := selectioncontract.ExecutableManifestSHA256(manifest)
	if err != nil {
		fail(err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		fail(err)
	}
	if len(entries) > 10_000 {
		fail(fmt.Errorf("too many execution attempts"))
	}
	observations := make([]functionalapi.Observation, 0)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data := read(filepath.Join(directory, entry.Name()))
		document, decodeErr := contracts.DecodeExecutionAttemptV1(data)
		if decodeErr != nil {
			fail(decodeErr)
		}
		attempt, importErr := executioncontract.ImportAttemptV1(document)
		if importErr != nil {
			fail(importErr)
		}
		if attempt.Manifest.APIVersion != manifest.APIVersion || attempt.Manifest.SHA256 != manifestSHA {
			fail(fmt.Errorf("attempt %s uses another manifest", attempt.AttemptID))
		}
		observations = append(observations, observationsFromAttempt(attempt, expected)...)
	}

	return observations
}

func observationsFromAttempt(attempt execution.Attempt, expected map[string]contracts.HeadInventoryTest) []functionalapi.Observation {
	observations := make([]functionalapi.Observation, 0, len(attempt.Results))
	for _, result := range attempt.Results {
		key := attempt.TestRepository.Identity.ProviderRepositoryID + "\x00" + result.SuiteKey + "\x00" + result.TestKey
		test, exists := expected[key]
		if !exists || test.Revision.Algorithm != string(attempt.TestRevision.Algorithm) ||
			test.Revision.Digest != attempt.TestRevision.Digest || test.Adapter != attempt.AdapterID ||
			test.TestRepository.Provider != string(attempt.TestRepository.Identity.Provider) ||
			test.TestRepository.Host != attempt.TestRepository.Identity.Host ||
			test.TestRepository.Owner != attempt.TestRepository.Owner ||
			test.TestRepository.Name != attempt.TestRepository.Name {
			fail(fmt.Errorf("attempt %s differs from reviewed head inventory", attempt.AttemptID))
		}
		observations = append(observations, functionalapi.Observation{
			RepositoryID: attempt.TestRepository.Identity.ProviderRepositoryID,
			SuiteKey:     result.SuiteKey, TestKey: result.TestKey,
			AttemptID: attempt.AttemptID, Outcome: string(result.Outcome),
		})
	}

	return observations
}

func read(path string) []byte {
	info, err := os.Stat(path)
	if err != nil {
		fail(err)
	}
	if info.Size() > 16<<20 {
		fail(fmt.Errorf("input exceeds 16 MiB"))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fail(err)
	}

	return data
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
