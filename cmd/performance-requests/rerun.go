package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/stanimirivanov/argus/internal/performance/adapters/cli/requestcli"
	"github.com/stanimirivanov/argus/internal/performance/adapters/perfeng"
	"github.com/stanimirivanov/argus/internal/performance/adapters/selectionio"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
	"github.com/stanimirivanov/argus/internal/postgres"
)

// The authenticated owner/CI host supplies this file outside submitted input
// control. Empty RerunSHA256 grants inspection only, never creation authority.
type rerunApproval struct {
	Request selectionio.RequestApproval `json:"request"`
	Parents []rerunParentPin            `json:"parents"`
}
type rerunParentPin struct {
	Position     int64  `json:"position"`
	ReviewSHA256 string `json:"reviewSha256"`
	Ordinal      int    `json:"ordinal"`
	RerunSHA256  string `json:"rerunSha256,omitempty"`
}

func decodeRerunApproval(raw []byte, write bool) (rerunApproval, error) {
	var a rerunApproval
	if len(raw) > 16<<20 || !utf8.Valid(raw) || json.Unmarshal(raw, &a) != nil {
		return a, requestintent.ErrInvalid
	}
	canonical, err := json.Marshal(a)
	if err != nil || !bytes.Equal(bytes.TrimSpace(raw), canonical) || len(a.Parents) == 0 || len(a.Parents) > 1000 || !requestintent.ValidScope(a.scope()) {
		return rerunApproval{}, requestintent.ErrInvalid
	}
	for _, issuer := range []string{a.Request.Expected.Issuer, a.Request.Expected.PrincipalIssuer} {
		parsed, err := url.Parse(issuer)
		if err != nil || !parsed.IsAbs() || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return rerunApproval{}, requestintent.ErrInvalid
		}
	}
	positions, ordinals := map[int64]bool{}, map[int]bool{}
	for _, pin := range a.Parents {
		if pin.Position <= 0 || pin.Ordinal < 0 || pin.Ordinal >= 1000 || (write && pin.Ordinal >= len(a.Parents)) || positions[pin.Position] || ordinals[pin.Ordinal] || !rerunDigest(pin.ReviewSHA256) || (pin.RerunSHA256 != "" && !rerunDigest(pin.RerunSHA256)) || (write && pin.RerunSHA256 == "") {
			return rerunApproval{}, requestintent.ErrInvalid
		}
		positions[pin.Position], ordinals[pin.Ordinal] = true, true
	}
	if write && !rerunDigest(a.Request.Authority.ReviewSHA256) {
		return rerunApproval{}, requestintent.ErrInvalid
	}

	return a, nil
}

func (a rerunApproval) scope() requestintent.Scope {
	return requestintent.Scope{Issuer: a.Request.Expected.Issuer, PrincipalIssuer: a.Request.Expected.PrincipalIssuer, PrincipalID: a.Request.Expected.PrincipalID}
}
func rerunDigest(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == 32 && strings.ToLower(value) == value
}

type rerunStorage interface {
	GetRequest(context.Context, requestintent.Scope, string) (requestintent.Intent, error)
	PendingRequests(context.Context, requestintent.Scope, int64, int) ([]requestintent.Pending, error)
	RequestOutcome(context.Context, requestintent.Scope, int64) (requestintent.Outcome, error)
	LifecycleHistory(context.Context, requestintent.Scope, int64, requestintent.LifecycleAdmission, requestintent.LifecycleVerifier) ([]requestintent.LifecycleEvent, error)
	CreateReruns(context.Context, requestintent.Intent, []requestintent.RerunCreation) (bool, error)
}

type lineageStorage interface {
	GetRequest(context.Context, requestintent.Scope, string) (requestintent.Intent, error)
	RerunSuccessor(context.Context, requestintent.Scope, int64) (requestintent.RerunRecord, error)
}

func rerunOperation(ctx context.Context, request requestcli.Creation) (requestintent.Intent, bool, error) {
	raw, err := requestcli.ReadCreationFile(os.Getenv("ARGUS_PERFORMANCE_RERUN_APPROVAL_FILE"))
	if err != nil {
		return requestintent.Intent{}, false, err
	}
	a, err := decodeRerunApproval(raw, true)
	if err != nil {
		return requestintent.Intent{}, false, err
	}
	review, err := selectionio.DecodeRequestReview(request.Review)
	if err != nil {
		return requestintent.Intent{}, false, err
	}
	canonical, err := json.Marshal(review)
	if err != nil {
		return requestintent.Intent{}, false, requestintent.ErrInvalid
	}
	digest := sha256.Sum256(canonical)
	if hex.EncodeToString(digest[:]) != a.Request.Authority.ReviewSHA256 {
		return requestintent.Intent{}, false, requestintent.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return requestintent.Intent{}, false, err
	}
	runtime, err := postgres.OpenRuntime(ctx, os.Getenv("ARGUS_DATABASE_URL"))
	if err != nil {
		return requestintent.Intent{}, false, err
	}
	defer runtime.Close()
	store := runtime.Performance()

	return createStored(ctx, request, review, a.Request, creationStorage{find: runtime.Change().FindCapabilityImpact, current: store.Current, create: func(ctx context.Context, intent requestintent.Intent) (bool, error) {
		return createApprovedRerun(ctx, intent, a, store)
	}})
}

func createApprovedRerun(ctx context.Context, successor requestintent.Intent, a rerunApproval, store rerunStorage) (bool, error) {
	if successor.Data().Scope != a.scope() || len(successor.Data().Children) != len(a.Parents) {
		return false, requestintent.ErrInvalid
	}
	bindings := make([]requestintent.RerunCreation, len(a.Parents))
	for i, pin := range a.Parents {
		binding, err := approvedRerunParent(ctx, successor, a.scope(), pin, store)
		if err != nil {
			return false, err
		}
		bindings[i] = binding
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}

	return store.CreateReruns(ctx, successor, bindings)
}

func approvedRerunParent(ctx context.Context, successor requestintent.Intent, scope requestintent.Scope, pin rerunParentPin, store rerunStorage) (requestintent.RerunCreation, error) {
	previous, err := store.GetRequest(ctx, scope, pin.ReviewSHA256)
	if err != nil {
		return requestintent.RerunCreation{}, err
	}
	pending, err := store.PendingRequests(ctx, scope, pin.Position-1, 1)
	if err != nil {
		return requestintent.RerunCreation{}, err
	}
	if len(pending) != 1 || pending[0].After != pin.Position {
		return requestintent.RerunCreation{}, requestintent.ErrNotFound
	}
	child := pending[0].Child
	original, err := perfeng.DecodeIntegration(child.Payload)
	if err != nil || original.Data.Context.Principal != (perfeng.IntegrationIdentity{Issuer: scope.PrincipalIssuer, ID: scope.PrincipalID}) {
		return requestintent.RerunCreation{}, requestintent.ErrInvalid
	}
	verify := func(outcome requestintent.Outcome) (requestintent.LifecycleAdmission, error) {
		return perfeng.BindLifecycleAdmission(child.Payload, outcome, scope.Issuer)
	}
	outcome, err := store.RequestOutcome(ctx, scope, pin.Position)
	if err != nil {
		return requestintent.RerunCreation{}, err
	}
	admission, err := verify(outcome)
	if err != nil {
		return requestintent.RerunCreation{}, err
	}
	history, err := store.LifecycleHistory(ctx, scope, pin.Position, admission, verify)
	if err != nil {
		return requestintent.RerunCreation{}, err
	}
	parent := requestintent.RerunParent{Pending: pending[0], Intent: previous, Admission: admission, History: history}
	proof, err := requestintent.NewRerun(parent, successor, pin.Ordinal)
	if err != nil || pin.RerunSHA256 == "" || proof.Fingerprint() != pin.RerunSHA256 {
		return requestintent.RerunCreation{}, requestintent.ErrConflict
	}

	return requestintent.RerunCreation{Parent: parent, Evidence: proof, Verify: verify}, nil
}

func lineageOperation(ctx context.Context, position int64) (requestintent.RerunRecord, error) {
	raw, err := requestcli.ReadCreationFile(os.Getenv("ARGUS_PERFORMANCE_RERUN_APPROVAL_FILE"))
	if err != nil {
		return requestintent.RerunRecord{}, err
	}
	a, err := decodeRerunApproval(raw, false)
	if err != nil {
		return requestintent.RerunRecord{}, err
	}
	var selected *rerunParentPin
	for i := range a.Parents {
		if a.Parents[i].Position == position {
			selected = &a.Parents[i]
		}
	}
	if selected == nil {
		return requestintent.RerunRecord{}, requestintent.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return requestintent.RerunRecord{}, err
	}
	runtime, err := postgres.OpenRuntime(ctx, os.Getenv("ARGUS_DATABASE_URL"))
	if err != nil {
		return requestintent.RerunRecord{}, err
	}
	defer runtime.Close()

	return lineageStored(ctx, a.scope(), *selected, runtime.Performance())
}

func lineageStored(ctx context.Context, scope requestintent.Scope, pin rerunParentPin, store lineageStorage) (requestintent.RerunRecord, error) {
	previous, err := store.GetRequest(ctx, scope, pin.ReviewSHA256)
	if err != nil {
		return requestintent.RerunRecord{}, err
	}
	record, err := store.RerunSuccessor(ctx, scope, pin.Position)
	if err != nil {
		return requestintent.RerunRecord{}, err
	}
	d := record.Evidence.Data()
	if d.Scope != scope || d.ParentPosition != pin.Position || d.ParentIntentSHA256 != previous.Fingerprint() || (pin.RerunSHA256 != "" && record.Evidence.Fingerprint() != pin.RerunSHA256) {
		return requestintent.RerunRecord{}, requestintent.ErrConflict
	}

	return record, nil
}
