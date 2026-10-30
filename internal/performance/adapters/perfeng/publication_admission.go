package perfeng

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"time"

	"github.com/stanimirivanov/argus/internal/performance"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

// PublicationAdmissionData preserves exact publication bytes/digest and the
// adapter-checked frozen request/admission pins. Native fields in Payload remain
// claims: this value supplies neither native review nor artifact-fetch authority.
type PublicationAdmissionData struct {
	Admission requestintent.LifecycleAdmission
	Payload   []byte
	SHA256    string
}

// PublicationAdmission owns admitted-request correlation, not decision evidence.
// Only BindPublicationAdmission creates a valid value. Storage/consumers must
// compare Admission with authoritative scoped rows before accepting this evidence.
type PublicationAdmission struct{ data PublicationAdmissionData }

// Data returns owned payload bytes and immutable admission evidence. A zero
// value has no digest; callers must reject it. Values support concurrent reads.
func (bound PublicationAdmission) Data() PublicationAdmissionData {
	data := bound.data
	data.Payload = bytes.Clone(data.Payload)

	return data
}

// BindPublicationAdmission checks full original context/public Run against the
// authoritative retained settlement and independently authenticated provider and
// principal. Body echoes grant no authentication. Inputs must not be mutated
// concurrently. Failure returns zero evidence; no I/O, clock or lifecycle change
// occurs. Native/policy/rule verification and exact artifact bytes remain required.
func BindPublicationAdmission(frozen []byte, retained requestintent.Outcome, raw []byte, provider string, principal performance.Identity) (PublicationAdmission, error) {
	if len(frozen) > IntegrationMaximumBytes || len(raw) > PublicationMaximumBytes {
		return PublicationAdmission{}, ErrPublication
	}
	frozen, raw = bytes.Clone(frozen), bytes.Clone(raw)
	message, err := DecodePublication(raw)
	if err != nil {
		return PublicationAdmission{}, err
	}
	original, err := DecodeIntegration(frozen)
	if err != nil || original.Type != "dev.perfeng.integration.requested.v1" || message.Source != provider ||
		performance.Identity(message.Data.Context.Principal) != principal || !reflect.DeepEqual(message.Data.Context, original.Data.Context) {
		return PublicationAdmission{}, ErrPublication
	}
	admission, err := BindLifecycleAdmission(frozen, retained, provider)
	if err != nil || admission.Data().Run != performance.Identity(message.Data.Run) {
		return PublicationAdmission{}, ErrPublication
	}
	settlement, err := DecodeIntegration(admission.Data().Evidence.Data().Payload)
	if err != nil {
		return PublicationAdmission{}, ErrPublication
	}
	if settlement.Type == "dev.perfeng.integration.accepted.v1" {
		accepted, acceptedErr := time.Parse(time.RFC3339Nano, settlement.Time)
		published, publishedErr := time.Parse(time.RFC3339Nano, message.Time)
		if acceptedErr != nil || publishedErr != nil || published.Before(accepted) {
			return PublicationAdmission{}, ErrPublication
		}
	}
	digest := sha256.Sum256(raw)

	return PublicationAdmission{data: PublicationAdmissionData{Admission: admission, Payload: raw, SHA256: hex.EncodeToString(digest[:])}}, nil
}
