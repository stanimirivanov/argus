package requestintent

import "time"

// Claim is an operational lease over owned immutable pending evidence. Token and
// monotonically increasing Fence identify this acquisition; Until is database time.
// A claim may expire before receipt. It grants neither submission eligibility nor
// approval; downstream writes must independently verify current live ownership.
type Claim struct {
	Pending Pending
	Token   string
	Fence   int64
	Until   time.Time
}
