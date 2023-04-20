package basics

var (
// _ msgp.Marshaler   = (*Trustworthiness)(nil)
// _ msgp.Unmarshaler = (*Trustworthiness)(nil)
// _ msgp.Sizer       = (*Trustworthiness)(nil)
)

const (
	minimalIncrease = 100
	increaseDivider = 10
)

type (
	Merger interface {
		Merge(algos MicroAlgos, scores Scores) uint64
	}
)

type SumMerger struct{}

func (_ SumMerger) Merge(algos MicroAlgos, scores Scores) uint64 {
	return algos.Raw + scores.Trustworthiness // TODO: find more generic way
}

type ConstantMerger struct {
	Total bool
}

func (m ConstantMerger) Merge(_ MicroAlgos, _ Scores) uint64 {
	if m.Total {
		return 1000000
	}
	return 250000
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
func (s Scores) IncreaseScores(highestStake, userStake MicroAlgos) Scores {
	// Trustworthiness
	ot := OverflowTracker{}
	deltaStake := ot.SubA(highestStake, userStake) // b_max - b_i
	if s.Trustworthiness > deltaStake.Raw {        // whether t_i > b_max - b_i
		// still apply small trustworthiness increase
		s.Trustworthiness = minimalIncrease
		return s
	}
	gain := ot.Sub(deltaStake.Raw, s.Trustworthiness) / increaseDivider
	s.Trustworthiness = max(gain, minimalIncrease) // if gain is too small, we use minimalIncrease
	return s
}

func (s Scores) Add(o Scores) Scores {
	s.Trustworthiness += o.Trustworthiness
	return s
}

func (s Scores) Sub(o Scores) Scores {
	s.Trustworthiness -= o.Trustworthiness
	return s
}

func max(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}

func findHighestBalance(accounts []AccountDetail) (highest AccountDetail) {
	for _, acc := range accounts {
		if acc.Algos.GreaterThan(highest.Algos) {
			highest = acc
		}
	}
	return
}
