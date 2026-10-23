package engine

import (
	"errors"
	"math"
)

// ErrUnknownProgram is returned by Simulate for an id that isn't a preset.
var ErrUnknownProgram = errors.New("unknown program")

// Preset is a built-in program for tab 1. Its step function maps the state x to
// the next state, or reports halted=true.
type Preset struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Sub   string  `json:"sub"`
	Desc  string  `json:"desc"`
	Code  string  `json:"code"`
	Start float64 `json:"start"`
	Limit int     `json:"limit"`
	step  func(x float64) (next float64, halted bool)
}

func countdown(x float64) (float64, bool) {
	if x > 0 {
		return x - 1, false
	}
	return x, true
}

var presets = []Preset{
	{ID: "countdown", Name: "Countdown", Sub: "Counts down to 0",
		Desc: "Subtracts 1 until x reaches 0. It always stops.",
		Code: "for x > 0 {\n\tx--\n}", Start: 8, Limit: 100, step: countdown},
	{ID: "up", Name: "Count up forever", Sub: "Never stops",
		Desc: "Adds 1 again and again. It never stops, but it also never repeats a value.",
		Code: "for {\n\tx++\n}", Start: 0, Limit: 100,
		step: func(x float64) (float64, bool) { return x + 1, false }},
	{ID: "flip", Name: "Flip-flop", Sub: "Repeats forever",
		Desc: "Switches x between 0 and 1. It never stops, and since it revisits the same value, we can prove that.",
		Code: "for {\n\tx = 1 - x\n}", Start: 0, Limit: 100,
		step: func(x float64) (float64, bool) { return 1 - x, false }},
	{ID: "slow", Name: "Slow countdown", Sub: "Stops, but slowly",
		Desc: "A countdown from a big number. With a small step limit, we give up too early.",
		Code: "for x > 0 {\n\tx--\n}", Start: 5000, Limit: 100, step: countdown},
	{ID: "collatz", Name: "Collatz (famous)", Sub: "Nobody knows for sure",
		Desc: "If x is even, halve it. If odd, triple it and add 1. Every number ever tested reaches 1, but nobody has proven it always does.",
		Code: "for x != 1 {\n\tif x%2 == 0 {\n\t\tx /= 2\n\t} else {\n\t\tx = 3*x + 1\n\t}\n}",
		Start: 27, Limit: 200,
		step: func(x float64) (float64, bool) {
			if x == 1 {
				return x, true
			}
			if math.Mod(x, 2) == 0 {
				return x / 2, false
			}
			return 3*x + 1, false
		}},
}

// Presets returns the built-in programs (a copy, safe to modify).
func Presets() []Preset {
	out := make([]Preset, len(presets))
	copy(out, presets)
	return out
}

// Simulate runs one preset from a start value under a step limit.
func Simulate(id string, start, limitRaw float64) (Result, error) {
	var p *Preset
	for i := range presets {
		if presets[i].ID == id {
			p = &presets[i]
		}
	}
	if p == nil {
		return Result{}, ErrUnknownProgram
	}
	x := math.Floor(start)
	if math.IsNaN(x) || math.IsInf(x, 0) || x < 0 {
		x = 0
	}
	if x > maxValue {
		x = maxValue
	}
	if id == "collatz" && x < 1 {
		x = 1
	}
	limit, adj := ClampLimit(limitRaw)
	res := Result{Limit: limit, LimitAdjusted: adj}
	seen := map[float64]int{}
	vals := []string{fmtNum(x)}
	steps := 0
	for {
		if first, ok := seen[x]; ok {
			res.Verdict = Loops
			res.FirstSeen = first
			res.RepeatedAt = steps
			res.State = fmtNum(x)
			break
		}
		seen[x] = steps
		nx, halted := p.step(x)
		if halted {
			res.Verdict = Halts
			break
		}
		if steps >= limit {
			res.Verdict = Unknown
			break
		}
		x = nx
		steps++
		vals = append(vals, fmtNum(x))
	}
	res.Steps = steps
	res.Trail, res.Elided = makeTrail(vals)
	return res, nil
}
