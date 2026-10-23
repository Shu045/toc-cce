package engine

import (
	"fmt"
	"go/ast"
	"go/token"
	"math"
	"strconv"
)

// A tiny slice of Go's type system: sized integers, float64 and bool.
// Integer arithmetic wraps exactly like real Go, so overflow is faithful.

type kind uint8

const (
	kInt kind = iota
	kFloat
	kBool
)

type typ struct {
	name   string
	k      kind
	bits   int
	signed bool
}

var (
	tInt    = &typ{"int", kInt, 64, true}
	tInt8   = &typ{"int8", kInt, 8, true}
	tInt16  = &typ{"int16", kInt, 16, true}
	tInt32  = &typ{"int32", kInt, 32, true}
	tInt64  = &typ{"int64", kInt, 64, true}
	tUint8  = &typ{"uint8", kInt, 8, false}
	tUint16 = &typ{"uint16", kInt, 16, false}
	tUint32 = &typ{"uint32", kInt, 32, false}
	tFloat  = &typ{"float64", kFloat, 0, false}
	tBool   = &typ{"bool", kBool, 0, false}

	typeByName = map[string]*typ{
		"int": tInt, "int8": tInt8, "int16": tInt16, "int32": tInt32, "rune": tInt32, "int64": tInt64,
		"uint8": tUint8, "byte": tUint8, "uint16": tUint16, "uint32": tUint32,
		"float64": tFloat, "bool": tBool,
	}
)

// value is one Go value. untyped marks constants such as 5, 2.5 or true, which
// adapt to the other operand's type like Go's untyped constants.
type value struct {
	t       *typ
	i       int64
	f       float64
	b       bool
	untyped bool
}

func iv(t *typ, i int64, un bool) value { return value{t: t, i: i, untyped: un} }
func fv(f float64, un bool) value       { return value{t: tFloat, f: f, untyped: un} }
func bv(b bool) value                   { return value{t: tBool, b: b, untyped: true} }

func zero(t *typ) value {
	switch t.k {
	case kInt:
		return iv(t, 0, false)
	case kFloat:
		return fv(0, false)
	}
	return value{t: tBool}
}

func (v value) String() string {
	switch v.t.k {
	case kInt:
		return strconv.FormatInt(v.i, 10)
	case kFloat:
		return strconv.FormatFloat(v.f, 'g', -1, 64)
	}
	if v.b {
		return "true"
	}
	return "false"
}

func (v value) appendKey(buf []byte) []byte {
	switch v.t.k {
	case kInt:
		buf = strconv.AppendInt(buf, v.i, 10)
	case kFloat:
		buf = strconv.AppendUint(buf, math.Float64bits(v.f), 16)
	default:
		if v.b {
			buf = append(buf, 't')
		} else {
			buf = append(buf, 'f')
		}
	}
	return append(buf, ',')
}

// wrap truncates v to the width of t, like a CPU register would.
func wrap(t *typ, v int64) int64 {
	if t.bits >= 64 {
		return v
	}
	if t.signed {
		sh := uint(64 - t.bits)
		return (v << sh) >> sh
	}
	return v & ((int64(1) << uint(t.bits)) - 1)
}

func fits(t *typ, v int64) bool { return wrap(t, v) == v }

func crash(msg string) *stop { return &stop{kind: Crash, msg: "panic: runtime error: " + msg} }

// convert adapts v to type t for assignment. Untyped constants convert freely
// when representable; typed values must already have type t.
func (in *interp) convert(n ast.Node, v value, t *typ) value {
	if !v.untyped {
		if v.t == t || (v.t.k == kBool && t.k == kBool) {
			return v
		}
		in.fail(n, "cannot use a value of type "+v.t.name+" as type "+t.name+" (mismatched types)")
	}
	switch t.k {
	case kFloat:
		if v.t.k == kInt {
			return fv(float64(v.i), false)
		}
		if v.t.k == kFloat {
			return fv(v.f, false)
		}
	case kInt:
		var i int64
		switch v.t.k {
		case kInt:
			i = v.i
		case kFloat:
			if v.f != math.Trunc(v.f) || math.IsInf(v.f, 0) {
				in.fail(n, fmt.Sprintf("constant %v truncated to integer", v.f))
			}
			i = int64(v.f)
		default:
			in.fail(n, "cannot use a boolean as "+t.name)
		}
		if !fits(t, i) {
			in.fail(n, fmt.Sprintf("constant %d overflows %s", i, t.name))
		}
		return iv(t, i, false)
	case kBool:
		if v.t.k == kBool {
			return bv(v.b)
		}
	}
	in.fail(n, "cannot use this value as type "+t.name)
	return value{}
}

// conv implements an explicit conversion such as int8(x) or float64(n).
func (in *interp) conv(n ast.Node, v value, t *typ) value {
	switch t.k {
	case kBool:
		if v.t.k == kBool {
			return bv(v.b)
		}
	case kFloat:
		if v.t.k == kInt {
			return fv(float64(v.i), false)
		}
		if v.t.k == kFloat {
			return fv(v.f, false)
		}
	case kInt:
		if v.t.k == kInt {
			if v.untyped && !fits(t, v.i) {
				in.fail(n, fmt.Sprintf("constant %d overflows %s", v.i, t.name))
			}
			return iv(t, wrap(t, v.i), false)
		}
		if v.t.k == kFloat {
			if v.untyped && v.f != math.Trunc(v.f) {
				in.fail(n, fmt.Sprintf("constant %v truncated to integer", v.f))
			}
			return iv(t, wrap(t, int64(v.f)), false)
		}
	}
	in.fail(n, "cannot convert this value to "+t.name)
	return value{}
}

// unify makes two operands the same type, following Go's constant rules.
func (in *interp) unify(n ast.Node, a, b value) (value, value) {
	switch {
	case a.untyped && !b.untyped:
		a = in.convert(n, a, b.t)
	case b.untyped && !a.untyped:
		b = in.convert(n, b, a.t)
	case a.untyped && b.untyped:
		if a.t.k == kFloat || b.t.k == kFloat {
			if a.t.k == kInt {
				a = fv(float64(a.i), true)
			}
			if b.t.k == kInt {
				b = fv(float64(b.i), true)
			}
		}
	default:
		if a.t != b.t && !(a.t.k == kBool && b.t.k == kBool) {
			in.fail(n, "invalid operation: mismatched types "+a.t.name+" and "+b.t.name)
		}
	}
	return a, b
}

func (in *interp) binop(n ast.Node, op token.Token, a, b value) value {
	if op == token.SHL || op == token.SHR {
		return in.shift(n, op, a, b)
	}
	a, b = in.unify(n, a, b)
	if a.t.k != b.t.k {
		in.fail(n, "invalid operation: mismatched types "+a.t.name+" and "+b.t.name)
	}
	un := a.untyped && b.untyped
	switch a.t.k {
	case kInt:
		x, y, t := a.i, b.i, a.t
		switch op {
		case token.ADD:
			return iv(t, wrap(t, x+y), un)
		case token.SUB:
			return iv(t, wrap(t, x-y), un)
		case token.MUL:
			return iv(t, wrap(t, x*y), un)
		case token.QUO:
			if y == 0 {
				panic(crash("integer divide by zero"))
			}
			return iv(t, wrap(t, x/y), un)
		case token.REM:
			if y == 0 {
				panic(crash("integer divide by zero"))
			}
			return iv(t, wrap(t, x%y), un)
		case token.AND:
			return iv(t, x&y, un)
		case token.OR:
			return iv(t, x|y, un)
		case token.XOR:
			return iv(t, wrap(t, x^y), un)
		case token.AND_NOT:
			return iv(t, x&^y, un)
		case token.EQL:
			return bv(x == y)
		case token.NEQ:
			return bv(x != y)
		case token.LSS:
			return bv(x < y)
		case token.LEQ:
			return bv(x <= y)
		case token.GTR:
			return bv(x > y)
		case token.GEQ:
			return bv(x >= y)
		}
	case kFloat:
		x, y := a.f, b.f
		switch op {
		case token.ADD:
			return fv(x+y, un)
		case token.SUB:
			return fv(x-y, un)
		case token.MUL:
			return fv(x*y, un)
		case token.QUO:
			return fv(x/y, un) // like Go: float division by zero gives ±Inf
		case token.EQL:
			return bv(x == y)
		case token.NEQ:
			return bv(x != y)
		case token.LSS:
			return bv(x < y)
		case token.LEQ:
			return bv(x <= y)
		case token.GTR:
			return bv(x > y)
		case token.GEQ:
			return bv(x >= y)
		case token.REM:
			in.fail(n, "operator % is not defined on float64")
		}
	case kBool:
		switch op {
		case token.EQL:
			return bv(a.b == b.b)
		case token.NEQ:
			return bv(a.b != b.b)
		}
	}
	in.fail(n, "the operator "+op.String()+" isn’t supported for "+a.t.name+" values")
	return value{}
}

func (in *interp) shift(n ast.Node, op token.Token, a, b value) value {
	if b.t.k == kFloat && b.untyped && b.f == math.Trunc(b.f) {
		b = iv(tInt, int64(b.f), true)
	}
	if a.t.k == kFloat && a.untyped && a.f == math.Trunc(a.f) {
		a = iv(tInt, int64(a.f), true)
	}
	if a.t.k != kInt || b.t.k != kInt {
		in.fail(n, "invalid shift: both operands must be integers")
	}
	if b.i < 0 {
		panic(crash("negative shift amount"))
	}
	var r int64
	if op == token.SHL {
		if b.i < 64 {
			r = a.i << uint(b.i)
		}
	} else if b.i >= 64 {
		if a.i < 0 {
			r = -1
		}
	} else {
		r = a.i >> uint(b.i)
	}
	return iv(a.t, wrap(a.t, r), a.untyped && b.untyped)
}
