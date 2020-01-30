package prober

// Result is the type for probe results.
type Result int

const (
	Unknown Result = iota - 1
	Success
	Failure
)

func (r Result) String() string {
	switch r {
	case Success:
		return "Success"
	case Failure:
		return "Failure"
	default:
		return "UNKNOWN"
	}
}
