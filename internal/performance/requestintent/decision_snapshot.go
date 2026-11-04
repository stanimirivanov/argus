package requestintent

// DecisionSnapshot contains owned verified retained evidence from one read-only
// snapshot. It is private application data, not an authorized public wire response.
// A zero Lifecycle denotes known absence of lifecycle observations.
type DecisionSnapshot struct {
	Intent    Intent
	Child     Child
	Admission LifecycleAdmission
	Lifecycle LifecycleEvent
	Analysis  AnalysisProjection
}

// AdmissionVerifier binds an authoritative settlement to the original frozen child.
// It must be pure and bounded; it grants no caller scope or submission authority.
type AdmissionVerifier func(Child, Outcome) (LifecycleAdmission, error)
