package layoutgraph

import "errors"

var (
	// ErrInvalidCandidate rejects a speculative layout candidate whose geometry
	// cannot be committed.
	ErrInvalidCandidate = errors.New("invalid layout candidate")
	// ErrNonImprovingCandidate rejects a valid candidate that does not improve
	// the current layout.
	ErrNonImprovingCandidate = errors.New("layout candidate did not improve")
)

// IsCandidateRejection reports whether err rejects only the current
// speculative candidate. Callers must propagate every other error.
func IsCandidateRejection(err error) bool {
	return errors.Is(err, ErrInvalidCandidate) || errors.Is(err, ErrNonImprovingCandidate)
}
