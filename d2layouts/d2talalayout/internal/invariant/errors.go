// Package invariant classifies violations of TALA's internal layout invariants.
package invariant

import (
	"errors"
	"fmt"
)

// ErrViolation identifies an internal layout invariant violation.
var ErrViolation = errors.New("layout invariant violated")

// New returns an invariant violation with the supplied reason.
func New(reason string) error {
	return fmt.Errorf("%w: %s", ErrViolation, reason)
}

// Errorf formats an invariant violation.
func Errorf(format string, args ...any) error {
	return New(fmt.Sprintf(format, args...))
}
