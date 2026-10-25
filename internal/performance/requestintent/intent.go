// Package requestintent owns immutable reviewed intent and pending-work vocabulary.
// Wire validation and authentication remain at driving boundaries; no I/O occurs here.
package requestintent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/stanimirivanov/argus/internal/performance"
)

// ErrInvalid denotes malformed inward evidence without reflecting provider content.
var ErrInvalid = errors.New("invalid performance request intent")

// Scope is the independently authorized provider/principal namespace.
type Scope struct{ Issuer, PrincipalIssuer, PrincipalID string }

// Reason retains every mapping/capability justification for a deduplicated child.
type Reason struct{ MappingID, Capability, Reason string }

// Child retains frozen correlation and exact adapter-validated interchange bytes.
type Child struct {
	Request, Attempt      performance.Identity
	EventID, SHA256       string
	RequestedAt, Deadline time.Time
	Payload               []byte
	Reasons               []Reason
}

// Data is the version-one fingerprint projection; changing its meaning requires
// compatibility review. Evidence retains original policy and canonical selection/review.
type Data struct {
	Scope                       Scope
	ReviewSHA256, CatalogSHA256 string
	Evidence                    []byte
	Children                    []Child
}

// Intent owns immutable inward evidence, not authentication or execution authority.
// Only New constructs valid values. Callers must not mutate New's inputs concurrently.
type Intent struct {
	data        Data
	fingerprint string
}

// New validates inward bounds and takes owned copies. Adapters must independently
// validate wire claims and approval before constructing these inward values.
func New(data Data) (Intent, error) {
	if !ValidScope(data.Scope) || !digest(data.ReviewSHA256) || !digest(data.CatalogSHA256) || !json.Valid(data.Evidence) || len(data.Evidence) > 64<<20 || len(data.Children) == 0 || len(data.Children) > 1000 {
		return Intent{}, ErrInvalid
	}
	data.Children = slices.Clone(data.Children)
	for i := range data.Children {
		c := &data.Children[i]
		c.RequestedAt, c.Deadline = c.RequestedAt.UTC(), c.Deadline.UTC()
		if !c.Valid() {
			return Intent{}, ErrInvalid
		}
	}
	raw, err := json.Marshal(struct {
		Version string
		Data    Data
	}{"argus.performance-request/v1", data})
	if err != nil {
		return Intent{}, ErrInvalid
	}
	intent := Intent{data: data, fingerprint: sum(raw)}
	intent.data = intent.Data()

	return intent, nil
}

// ValidScope rejects empty/unbounded identifiers before any storage operation.
func ValidScope(s Scope) bool {
	return text(s.Issuer, 512) && text(s.PrincipalIssuer, 512) && text(s.PrincipalID, 256)
}
func text(s string, maximum int) bool {
	// Match JSON Schema maxLength and PostgreSQL CHAR_LENGTH, not UTF-8 bytes.
	return utf8.ValidString(s) && strings.TrimSpace(s) != "" && utf8.RuneCountInString(s) <= maximum && !strings.ContainsRune(s, 0)
}
func sum(raw []byte) string { h := sha256.Sum256(raw); return hex.EncodeToString(h[:]) }
func digest(s string) bool {
	raw, err := hex.DecodeString(s)
	return err == nil && len(raw) == 32 && strings.ToLower(s) == s
}

// Valid checks inward identity/time bounds, owned payload integrity and reasons.
// It does not validate the provider protocol or authenticate any embedded claims.
func (c Child) Valid() bool {
	if !text(c.Request.Issuer, 512) || !text(c.Attempt.Issuer, 512) || !text(c.Request.ID, 256) || !text(c.Attempt.ID, 256) || !text(c.EventID, 256) || c.Request == c.Attempt || c.RequestedAt.IsZero() || !c.RequestedAt.Before(c.Deadline) || c.RequestedAt.Nanosecond()%1000 != 0 || c.Deadline.Nanosecond()%1000 != 0 || len(c.Payload) > 64<<10 || !json.Valid(c.Payload) || sum(c.Payload) != c.SHA256 || len(c.Reasons) == 0 || len(c.Reasons) > 1000 {
		return false
	}
	for _, reason := range c.Reasons {
		if !text(reason.MappingID, 256) || !text(reason.Capability, 256) || !text(reason.Reason, 256) {
			return false
		}
	}

	return true
}

// Fingerprint binds the complete versioned immutable evidence, including child order.
func (i Intent) Fingerprint() string { return i.fingerprint }

// Data returns owned copies safe to mutate; immutable Intent values are concurrent-readable.
func (i Intent) Data() Data {
	d := i.data
	d.Evidence = slices.Clone(d.Evidence)
	d.Children = slices.Clone(d.Children)
	for j := range d.Children {
		d.Children[j].Payload = slices.Clone(d.Children[j].Payload)
		d.Children[j].Reasons = slices.Clone(d.Children[j].Reasons)
	}

	return d
}
