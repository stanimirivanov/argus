package requestintent

import "github.com/stanimirivanov/argus/internal/performance"

// LifecycleAdmissionData binds retained admission to a provider run and frozen
// request. Initial is absent for acceptance, required for lifecycle settlement.
type LifecycleAdmissionData struct {
	Evidence      Outcome
	Run           performance.Identity
	RequestSHA256 string
	Initial       LifecycleEvent
}

// LifecycleAdmission owns adapter-validated admission correlation. It is evidence,
// not authentication; storage must compare it with the authoritative settlement.
type LifecycleAdmission struct{ data LifecycleAdmissionData }

// NewLifecycleAdmission checks inward consistency. The driving adapter must prove
// that Run and Initial come from Evidence's canonical payload and frozen context.
func NewLifecycleAdmission(data LifecycleAdmissionData) (LifecycleAdmission, error) {
	if data.Evidence.fingerprint == "" || !digest(data.RequestSHA256) || data.Run.Issuer != data.Evidence.data.Source || !text(data.Run.ID, 256) {
		return LifecycleAdmission{}, ErrInvalid
	}
	switch data.Evidence.data.Kind {
	case "accepted":
		if data.Initial.data.Evidence.fingerprint != "" {
			return LifecycleAdmission{}, ErrInvalid
		}
	case "lifecycle":
		initial := data.Initial.data
		if initial.Evidence.fingerprint != data.Evidence.fingerprint || initial.Run != data.Run || initial.RequestSHA256 != data.RequestSHA256 {
			return LifecycleAdmission{}, ErrInvalid
		}
	default:
		return LifecycleAdmission{}, ErrInvalid
	}

	return LifecycleAdmission{data: data}, nil
}

// Data returns immutable values whose evidence accessors copy payloads. Callers
// may read concurrently; no credentials or storage authority are retained.
func (admission LifecycleAdmission) Data() LifecycleAdmissionData { return admission.data }
