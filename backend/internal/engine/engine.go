// Package engine analyses small Go programs and preset programs.
// User code is parsed with the standard library (go/parser) and run by a
// sandboxed tree-walking interpreter over go/ast. It is never compiled or
// executed natively, and every run has a step limit.
package engine

import (
	"fmt"
	"math"
	"strconv"
)

// Verdict is the outcome of a bounded run.
type Verdict string

const (
	Halts    Verdict = "halts"    // the program finished
	Loops    Verdict = "loops"    // an exact state repeated: provably infinite
	Unknown  Verdict = "unknown"  // step limit reached, nothing repeated
	Crash    Verdict = "crash"    // runtime panic (e.g. integer divide by zero)
)

const (
	DefaultLimit = 10000
	MaxLimit     = 100000
	maxValue     = 1e15 // beyond this float64 loses integer precision
	trailHead    = 12
	trailTail    = 3
)

// CompileError is returned for code the tool can't parse or doesn't support.
type CompileError struct {
	Line int
	Msg  string
}

func (e *CompileError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("Line %d: %s", e.Line, e.Msg)
	}
	return e.Msg
}

// Result is what the API sends back to the frontend.
type Result struct {
	Verdict       Verdict  `json:"verdict"`
	Steps         int      `json:"steps"`
	Limit         int      `json:"limit"`
	LimitAdjusted bool     `json:"limitAdjusted"`
	Final         string   `json:"final,omitempty"`
	Message       string   `json:"message,omitempty"`
	LoopLine      int      `json:"loopLine,omitempty"`
	FirstSeen     int      `json:"firstSeen,omitempty"`
	RepeatedAt    int      `json:"repeatedAt,omitempty"`
	State         string   `json:"state,omitempty"`
	Trail         []string `json:"trail"`
	Elided        int      `json:"elided"`
	Notes         []string `json:"notes,omitempty"`
}

// ClampLimit maps any requested step limit into 1..MaxLimit.
func ClampLimit(v float64) (limit int, adjusted bool) {
	if math.IsNaN(v) || v < 1 {
		return DefaultLimit, true
	}
	if v > MaxLimit {
		return MaxLimit, true
	}
	return int(v), false
}

func fmtNum(v float64) string {
	if v == math.Trunc(v) && math.Abs(v) < maxValue {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// makeTrail keeps the first 12 and last 3 entries; elided is how many were dropped.
func makeTrail(items []string) (out []string, elided int) {
	if len(items) <= trailHead+trailTail+1 {
		if items == nil {
			items = []string{}
		}
		return items, 0
	}
	out = append(out, items[:trailHead]...)
	out = append(out, items[len(items)-trailTail:]...)
	return out, len(items) - trailHead - trailTail
}
