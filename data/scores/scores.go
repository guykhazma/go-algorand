package scores

import (
	"github.com/algorand/go-algorand/data/basics"
)

var (
// _ msgp.Marshaler   = (*Trustworthiness)(nil)
// _ msgp.Unmarshaler = (*Trustworthiness)(nil)
// _ msgp.Sizer       = (*Trustworthiness)(nil)
)

type (
	Merger interface {
		Merge(algos basics.MicroAlgos, scores Scores) uint64
	}
)

type SumMerger struct{}

func (_ SumMerger) Merge(algos basics.MicroAlgos, scores Scores) uint64 {
	return algos.Raw + scores.Trustworthiness // TODO: find more generic way
}

// Scores contains different kinds of selection score that are used to make the
// user distribution during the sortition algorithm uniform.
type Scores struct {
	_struct         struct{} `codec:",omitempty,omitemptyarray"`
	Trustworthiness uint64   `codec:"trustworthiness"`
}

func (s Scores) IsEmpty() bool {
	return s.Trustworthiness == 0
}

// IncreaseScores increases scores values and returns the updates values.
func (s Scores) IncreaseScores() Scores {
	// Trustworthiness
	s.Trustworthiness += 100
	return s
}
