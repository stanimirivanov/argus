package replaycli_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/predictive/adapters/replaycli"
)

func calibrationArchive(t *testing.T, directory string) contracts.PredictiveDatasetIndexV1 {
	t.Helper()
	index := rankingArchive(t, directory)
	calibration := observation(t, 45, "tests-a", "full-d", "2026-10-20T09:45:00Z")
	calibration.Observation.Tests = make([]contracts.PredictiveTestV1, 0, 22)
	for i := range 22 {
		outcome := "passed"
		if i%5 == 0 {
			outcome = "failed"
		}
		calibration.Observation.Tests = append(calibration.Observation.Tests, contracts.PredictiveTestV1{
			SuiteKey: "api", TestKey: fmt.Sprintf("cal-%02d", i), Decision: "SKIP_FOR_NOW",
			Reasons: []string{"NOT_AFFECTED_IN_EARLY_STAGE"}, Capabilities: []string{}, MappingKeys: []string{},
			Outcome: outcome, DurationMS: 100})
	}
	var err error
	calibration, err = contracts.NewPredictiveObservationEnvelopeV1(calibration.Observation)
	if err != nil {
		t.Fatal(err)
	}
	save(t, directory, "calibration.json", calibration)
	index.Episodes = append(index.Episodes, contracts.PredictiveIndexEpisode{
		ID: "calibration", Observations: []string{"calibration.json"}})

	return index
}

func TestCalibrationReportIsPortableAndOrderIndependent(t *testing.T) {
	directory := t.TempDir()
	index := calibrationArchive(t, directory)
	run := func() ([]byte, contracts.PredictiveCalibrationV1) {
		t.Helper()
		save(t, directory, "index.json", index)
		var output bytes.Buffer
		if err := replaycli.RunCalibration([]string{"-index", filepath.Join(directory, "index.json")}, &output); err != nil {
			t.Fatal(err)
		}
		var report contracts.PredictiveCalibrationV1
		if err := json.Unmarshal(output.Bytes(), &report); err != nil {
			t.Fatal(err)
		}
		if err := contracts.ValidatePredictiveCalibrationV1(report); err != nil {
			t.Fatal(err)
		}

		return output.Bytes(), report
	}
	first, report := run()
	index.Episodes[0], index.Episodes[3] = index.Episodes[3], index.Episodes[0]
	second, _ := run()
	if !bytes.Equal(first, second) || len(report.FitEpisodeIDs) != 2 ||
		len(report.CalibrationEpisodeIDs) != 1 || report.GlobalCurve.Examples != 22 ||
		len(report.Episodes) != 1 || len(report.Quality) < 1 {
		t.Fatalf("incomplete or order-dependent report: %+v", report)
	}
}

func TestCalibrationRejectsInsufficientControlsWithoutOutput(t *testing.T) {
	directory := t.TempDir()
	index := rankingArchive(t, directory)
	save(t, directory, "index.json", index)
	var output bytes.Buffer
	err := replaycli.RunCalibration([]string{"-index", filepath.Join(directory, "index.json")}, &output)
	if err == nil || output.Len() != 0 {
		t.Fatalf("insufficient calibration emitted output: err=%v bytes=%d", err, output.Len())
	}
}
