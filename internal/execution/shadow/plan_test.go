package shadow

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/execution/planning"
)

func TestComparePlanIncludesSelectedAndFullOnlyGroups(t *testing.T) {
	t.Parallel()
	plan, bindings, attempts := planShadowFixture()

	report, err := NewService(stubReader{attempts: attempts}).ComparePlan(
		t.Context(), plan, strings.Repeat("c", 64), bindings,
	)
	if err != nil {
		t.Fatalf("compare execution plan: %v", err)
	}
	if report.GroupCount != 2 || report.GroupsWithSelection != 1 || report.FullOnlyGroupCount != 1 ||
		report.SelectedTestCount != 1 || report.FullSuiteTestCount != 3 ||
		report.SelectedDurationMS != 1000 || report.FullSuiteDurationMS != 6000 ||
		report.DurationReductionMS != 5000 || report.FullSuiteFailureCount != 2 ||
		report.CaughtFullSuiteFailureCount != 1 || report.FailureRecallBasisPoints == nil ||
		*report.FailureRecallBasisPoints != 5000 {
		t.Fatalf("plan report = %+v", report)
	}
	if report.Groups[0].GroupKey != "orders-playwright" ||
		report.Groups[1].GroupKey != "payments-pytest" ||
		report.Groups[1].SelectedAttemptID != nil ||
		len(report.Groups[1].MissedFailures) != 1 ||
		report.Groups[1].MissedFailures[0].Reason != MissNotSelected ||
		report.Groups[1].FailureRecallBasisPoints == nil ||
		*report.Groups[1].FailureRecallBasisPoints != 0 {
		t.Fatalf("group reports = %+v", report.Groups)
	}
}

func TestComparePlanRequiresExactPlannedGroupBindings(t *testing.T) {
	t.Parallel()
	plan, bindings, attempts := planShadowFixture()
	service := NewService(stubReader{attempts: attempts})

	missing := bindings
	missing.Groups = missing.Groups[:1]
	if _, err := service.ComparePlan(t.Context(), plan, strings.Repeat("c", 64), missing); !errors.Is(err, execution.ErrInvalid) {
		t.Fatalf("missing group error = %v", err)
	}

	unexpectedSelected := bindings
	selectedID := "unexpected-selected"
	unexpectedSelected.Groups[0].SelectedAttemptID = &selectedID
	if _, err := service.ComparePlan(
		t.Context(), plan, strings.Repeat("c", 64), unexpectedSelected,
	); !errors.Is(err, execution.ErrInvalid) {
		t.Fatalf("unexpected selected attempt error = %v", err)
	}
}

func TestComparePlanRejectsAttemptThatDoesNotSatisfyJob(t *testing.T) {
	t.Parallel()
	plan, bindings, attempts := planShadowFixture()
	payment := attempts["payments-full"]
	payment.Results = append(payment.Results, result("payments", "capture", execution.TestPassed, time.Second))
	payment.Outcome = execution.DeriveAttemptOutcome(payment.Results)
	attempts[payment.AttemptID] = payment

	_, err := NewService(stubReader{attempts: attempts}).ComparePlan(
		t.Context(), plan, strings.Repeat("c", 64), bindings,
	)
	if !errors.Is(err, execution.ErrInvalid) {
		t.Fatalf("attempt count mismatch error = %v", err)
	}
}

func TestValidatePlanAttemptBindingsRejectsReusedAttempt(t *testing.T) {
	t.Parallel()
	_, bindings, _ := planShadowFixture()
	bindings.Groups[1].FullSuiteAttemptID = bindings.Groups[0].FullSuiteAttemptID
	if err := ValidatePlanAttemptBindings(bindings); !errors.Is(err, execution.ErrInvalid) {
		t.Fatalf("duplicate attempt error = %v", err)
	}
}

func planShadowFixture() (planning.Plan, PlanAttemptBindings, map[string]execution.Attempt) {
	ordersSelected := shadowAttempt(execution.StageSelected, "orders-selected", []execution.TestResult{
		result("orders", "create", execution.TestFailed, time.Second),
	})
	ordersFull := shadowAttempt(execution.StageFullSuite, "orders-full", []execution.TestResult{
		result("orders", "create", execution.TestFailed, 2*time.Second),
		result("orders", "list", execution.TestPassed, time.Second),
	})
	paymentsRepository := catalog.Repository{
		Identity: catalog.RepositoryIdentity{
			Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "tests-2",
		},
		Owner: "example", Name: "payments-tests",
	}
	paymentsFull := shadowAttempt(execution.StageFullSuite, "payments-full", []execution.TestResult{
		result("payments", "refund", execution.TestFailed, 3*time.Second),
	})
	paymentsFull.TestRepository = paymentsRepository
	paymentsFull.AdapterID = "pytest"
	paymentsFull.AdapterVersion = "9.1.0"
	paymentsFull.TestRevision = catalog.Revision{
		Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("d", 40),
	}

	plan := planning.Plan{
		APIVersion: planning.PlanAPIVersion, Manifest: ordersSelected.Manifest,
		Jobs: []planning.Job{
			{
				GroupKey: "payments-pytest", Stage: execution.StageFullSuite,
				TestRepository: paymentsRepository, TestRevision: paymentsFull.TestRevision,
				Adapter: "pytest", TestCount: 1,
			},
			{
				GroupKey: "orders-playwright", Stage: execution.StageFullSuite,
				TestRepository: ordersFull.TestRepository, TestRevision: ordersFull.TestRevision,
				Adapter: "playwright", TestCount: 2,
			},
			{
				GroupKey: "orders-playwright", Stage: execution.StageSelected,
				TestRepository: ordersSelected.TestRepository, TestRevision: ordersSelected.TestRevision,
				Adapter: "playwright", TestCount: 1,
			},
		},
	}
	selectedID := ordersSelected.AttemptID
	bindings := PlanAttemptBindings{
		APIVersion: PlanAttemptBindingsAPIVersion,
		Groups: []GroupAttemptBinding{
			{GroupKey: "payments-pytest", FullSuiteAttemptID: paymentsFull.AttemptID},
			{
				GroupKey: "orders-playwright", SelectedAttemptID: &selectedID,
				FullSuiteAttemptID: ordersFull.AttemptID,
			},
		},
	}
	attempts := map[string]execution.Attempt{
		ordersSelected.AttemptID: ordersSelected,
		ordersFull.AttemptID:     ordersFull,
		paymentsFull.AttemptID:   paymentsFull,
	}

	return plan, bindings, attempts
}
