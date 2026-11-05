// Package ratelimit defines results shared by rate-limiting algorithms.
package ratelimit

import "time"

// Reason explains a rejection. Its zero value means no rejection.
type Reason string

const (
	ReasonRateLimited      Reason = "rate_limited"
	ReasonCapacityExceeded Reason = "capacity_exceeded"
	ReasonInvalidKey       Reason = "invalid_key"
)

// Decision is the result of evaluating one request.
type Decision struct {
	Allowed   bool
	Remaining int
	// RetryAfter is a hint for rate-limit rejections. Capacity rejections
	// return zero because no retry time is promised.
	RetryAfter time.Duration
	Reason     Reason
}
