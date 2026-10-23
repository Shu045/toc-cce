package engine

import (
	"fmt"
	"go/ast"
	"go/token"
	"math"
	"strconv"
	"strings"
)

// stop is panicked to end a run early; Run recovers it.
type stop struct {
	kind   Verdict
	msg    string
	loopID int
	first  int
	at     int
	state  string
}

type ctl int

const (
	ctlNone ctl = iota
	ctlBreak
	ctlContinue
	ctlReturn
)

// slot is one variable. Hidden slots (name "") hold loop bookkeeping that is
// part of the program state but not shown to the user.
type slot struct {
	name string
	v    value
}

type interp struct {
	prog  *Program
	vars  []slot
	marks []int // start index of each open scope in vars
	limit int
	steps int
	hooks int
	seen  map[string]int
	hist  []string
}

func (in *interp) fail(n ast.Node, msg string) {
	panic(&CompileError{Line: in.prog.line(n.Pos()), Msg: msg})
}

func (in *interp) push() { in.marks = append(in.marks, len(in.vars)) }

func (in *interp) pop() {
	m := in.marks[len(in.marks)-1]
	in.marks = in.marks[:len(in.marks)-1]
	in.vars = in.vars[:m]
}

func (in *interp) declare(name string, v value) { in.vars = append(in.vars, slot{name: name, v: v}) }

func (in *interp) lookup(name string) *slot {
	for i := len(in.vars) - 1; i >= 0; i-- {
		if in.vars[i].name == name {
			return &in.vars[i]
		}
	}
	return nil
}

func (in *interp) inScope(name string) *slot {
	start := 0
	if len(in.marks) > 0 {
		start = in.marks[len(in.marks)-1]
	}
	for i := len(in.vars) - 1; i >= start; i-- {
		if in.vars[i].name == name {
			return &in.vars[i]
		}
	}
	return nil
}

func (in *interp) tick() {
	in.steps++
	if in.steps > in.limit {
		panic(&stop{kind: Unknown})
	}
}

func (in *interp) label() string {
	var sb strings.Builder
	first := true
	for _, s := range in.vars {
		if s.name == "" {
			continue
		}
		if !first {
			sb.WriteString(", ")
		}
		first = false
		sb.WriteString(s.name)
		sb.WriteByte('=')
		sb.WriteString(s.v.String())
	}
	return sb.String()
}

// key identifies the complete program state at a loop head: which loop plus
// every live variable (including hidden loop counters). Go is deterministic,
// so a repeated key means the same future: the program loops forever.
func (in *interp) key(id int) string {
	buf := make([]byte, 0, 64)
	buf = strconv.AppendInt(buf, int64(id), 10)
	buf = append(buf, '|')
	for i := range in.vars {
		buf = in.vars[i].v.appendKey(buf)
	}
	return string(buf)
}

func (in *interp) hook(id int) {
	k := in.key(id)
	in.hooks++
	lab := in.label()
	in.hist = append(in.hist, lab)
	if p, ok := in.seen[k]; ok {
		panic(&stop{kind: Loops, loopID: id, first: p, at: in.hooks, state: lab})
	}
	in.seen[k] = in.hooks
}

// ---------- running ----------

// Run executes the program with a step limit. A non-nil error means the code
// has a Go type or name error that only shows up when that line runs.
func (p *Program) Run(limitRaw float64) (res Result, err error) {
	limit, adj := ClampLimit(limitRaw)
	in := &interp{prog: p, limit: limit, seen: map[string]int{}}
	defer func() {
		if x := recover(); x != nil {
			switch s := x.(type) {
			case *stop:
				res = in.finish(s)
				res.LimitAdjusted = adj
				err = nil
			case *CompileError:
				res = Result{}
				err = s
			default:
				panic(x)
			}
		}
	}()
	for _, d := range p.file.Decls {
		if g, ok := d.(*ast.GenDecl); ok && (g.Tok == token.VAR || g.Tok == token.CONST) {
			in.decl(g)
		}
	}
	in.push() // main's scope stays open so the final values can be reported
	in.execList(p.main.Body.List)
	res = in.finish(nil)
	res.LimitAdjusted = adj
	return res, nil
}

func (in *interp) finish(s *stop) Result {
	p := in.prog
	res := Result{Steps: in.steps, Limit: in.limit, Verdict: Halts, Notes: p.notes()}
	hist := in.hist
	if s == nil {
		res.Final = in.label()
		hist = append(hist, res.Final)
	} else {
		res.Verdict = s.kind
		switch s.kind {
		case Loops:
			res.LoopLine = p.loops[s.loopID-1].line
			res.FirstSeen = s.first
			res.RepeatedAt = s.at
			res.State = s.state
		case Crash:
			res.Message = s.msg
		}
	}
	res.Trail, res.Elided = makeTrail(hist)
	return res
}

// ---------- statements ----------

func (in *interp) execList(list []ast.Stmt) ctl {
	for _, s := range list {
		if c := in.exec(s); c != ctlNone {
			return c
		}
	}
	return ctlNone
}

func (in *interp) execBlock(b *ast.BlockStmt) ctl {
	in.push()
	c := in.execList(b.List)
	in.pop()
	return c
}

func (in *interp) needBool(v value, n ast.Node) {
	if v.t.k != kBool {
		in.fail(n, "a condition must be true or false (a bool), not a number")
	}
}

func (in *interp) exec(s ast.Stmt) ctl {
	switch x := s.(type) {
	case *ast.BlockStmt:
		return in.execBlock(x)
	case *ast.ExprStmt:
		if c, ok := x.X.(*ast.CallExpr); ok && isFmt(c) {
			return ctlNone
		}
		in.eval(x.X)
		return ctlNone
	case *ast.AssignStmt:
		in.assign(x)
		return ctlNone
	case *ast.IncDecStmt:
		id := in.ident(x.X)
		sl := in.lookup(id.Name)
		if sl == nil {
			in.fail(id, "undefined: "+id.Name)
		}
		op := token.ADD
		if x.Tok == token.DEC {
			op = token.SUB
		}
		res := in.binop(x, op, sl.v, iv(tInt, 1, true))
		sl.v = in.convert(x, res, sl.v.t)
		return ctlNone
	case *ast.IfStmt:
		in.push()
		if x.Init != nil {
			in.exec(x.Init)
		}
		cond := in.eval(x.Cond)
		in.needBool(cond, x.Cond)
		c := ctlNone
		if cond.b {
			c = in.execBlock(x.Body)
		} else if x.Else != nil {
			c = in.exec(x.Else)
		}
		in.pop()
		return c
	case *ast.ForStmt:
		return in.forLoop(x)
	case *ast.RangeStmt:
		return in.rangeLoop(x)
	case *ast.BranchStmt:
		if x.Tok == token.BREAK {
			return ctlBreak
		}
		return ctlContinue
	case *ast.ReturnStmt:
		return ctlReturn
	case *ast.DeclStmt:
		if g, ok := x.Decl.(*ast.GenDecl); ok {
			in.decl(g)
		}
		return ctlNone
	case *ast.EmptyStmt:
		return ctlNone
	}
	in.fail(s, unsupportedName(s)+" isn’t supported in this tool.")
	return ctlNone
}

func (in *interp) ident(e ast.Expr) *ast.Ident {
	id, ok := e.(*ast.Ident)
	if !ok {
		in.fail(e, "only simple variables can be assigned to")
	}
	return id
}

func (in *interp) store(n ast.Node, s *slot, v value) { s.v = in.convert(n, v, s.v.t) }

func (in *interp) assign(x *ast.AssignStmt) {
	if x.Tok == token.DEFINE || x.Tok == token.ASSIGN {
		vals := make([]value, len(x.Rhs))
		for i, e := range x.Rhs {
			vals[i] = in.eval(e)
		}
		anyNew := false
		for i, l := range x.Lhs {
			id := in.ident(l)
			if id.Name == "_" {
				continue
			}
			if x.Tok == token.DEFINE {
				if sl := in.inScope(id.Name); sl != nil {
					in.store(x, sl, vals[i])
				} else {
					v := vals[i]
					v.untyped = false
					in.declare(id.Name, v)
					anyNew = true
				}
			} else {
				sl := in.lookup(id.Name)
				if sl == nil {
					in.fail(id, "undefined: "+id.Name)
				}
				in.store(x, sl, vals[i])
			}
		}
		if x.Tok == token.DEFINE && !anyNew {
			in.fail(x, "no new variables on the left side of :=")
		}
		return
	}
	id := in.ident(x.Lhs[0])
	sl := in.lookup(id.Name)
	if sl == nil {
		in.fail(id, "undefined: "+id.Name)
	}
	r := in.eval(x.Rhs[0])
	res := in.binop(x, assignOps[x.Tok], sl.v, r)
	in.store(x, sl, res)
}

func (in *interp) decl(d *ast.GenDecl) {
	for _, sp := range d.Specs {
		vs, ok := sp.(*ast.ValueSpec)
		if !ok {
			continue
		}
		var t *typ
		if vs.Type != nil {
			if id, ok := vs.Type.(*ast.Ident); ok {
				t = typeByName[id.Name]
			}
			if t == nil {
				in.fail(vs, "this type isn’t supported. Use int, int8–int64, uint8–uint32, float64 or bool.")
			}
		}
		var vals []value
		switch {
		case len(vs.Values) == 0:
			if t == nil {
				in.fail(vs, "a variable needs a type or a value")
			}
			for range vs.Names {
				vals = append(vals, zero(t))
			}
		case len(vs.Values) == len(vs.Names):
			for _, e := range vs.Values {
				v := in.eval(e)
				if t != nil {
					v = in.convert(vs, v, t)
				} else {
					v.untyped = false
				}
				vals = append(vals, v)
			}
		default:
			in.fail(vs, "the number of names and values doesn’t match")
		}
		for i, nm := range vs.Names {
			if nm.Name != "_" {
				in.declare(nm.Name, vals[i])
			}
		}
	}
}

func (in *interp) forLoop(x *ast.ForStmt) ctl {
	id := in.prog.ids[x]
	in.push()
	if x.Init != nil {
		in.exec(x.Init)
	}
	res := ctlNone
	for {
		in.hook(id)
		if x.Cond != nil {
			c := in.eval(x.Cond)
			in.needBool(c, x.Cond)
			if !c.b {
				break
			}
		}
		in.tick()
		c := in.execBlock(x.Body)
		if c == ctlBreak {
			break
		}
		if c == ctlReturn {
			res = ctlReturn
			break
		}
		if x.Post != nil {
			in.exec(x.Post)
		}
	}
	in.pop()
	return res
}

func (in *interp) rangeLoop(x *ast.RangeStmt) ctl {
	id := in.prog.ids[x]
	nv := in.eval(x.X)
	if nv.t.k == kFloat && nv.untyped && nv.f == math.Trunc(nv.f) {
		nv = iv(tInt, int64(nv.f), true)
	}
	if nv.t.k != kInt {
		in.fail(x.X, "range only works over a whole number here, for example  for i := range 10")
	}
	nv.untyped = false
	in.push()
	base := len(in.vars)
	in.declare("", nv)                 // hidden: the bound n
	in.declare("", iv(nv.t, 0, false)) // hidden: the counter
	hasKey := false
	if k, ok := x.Key.(*ast.Ident); ok && k.Name != "_" {
		in.declare(k.Name, iv(nv.t, 0, false))
		hasKey = true
	}
	res := ctlNone
	for i := int64(0); ; i++ {
		in.vars[base+1].v = iv(nv.t, i, false)
		in.hook(id)
		if i >= nv.i {
			break
		}
		in.tick()
		if hasKey {
			in.vars[base+2].v = iv(nv.t, i, false)
		}
		c := in.execBlock(x.Body)
		if c == ctlBreak {
			break
		}
		if c == ctlReturn {
			res = ctlReturn
			break
		}
	}
	in.pop()
	return res
}

// ---------- expressions ----------

func (in *interp) eval(e ast.Expr) value {
	switch x := e.(type) {
	case *ast.ParenExpr:
		return in.eval(x.X)
	case *ast.BasicLit:
		return in.lit(x)
	case *ast.Ident:
		switch x.Name {
		case "true":
			return bv(true)
		case "false":
			return bv(false)
		}
		if s := in.lookup(x.Name); s != nil {
			return s.v
		}
		in.fail(x, "undefined: "+x.Name)
	case *ast.UnaryExpr:
		return in.unaryExpr(x)
	case *ast.BinaryExpr:
		return in.binaryExpr(x)
	case *ast.CallExpr:
		return in.callExpr(x)
	case *ast.SelectorExpr:
		return in.selector(x)
	}
	in.fail(e, "this kind of expression isn’t supported")
	return value{}
}

func (in *interp) lit(x *ast.BasicLit) value {
	switch x.Kind {
	case token.INT:
		i, err := strconv.ParseInt(x.Value, 0, 64)
		if err != nil {
			in.fail(x, "the number "+x.Value+" is too large")
		}
		return iv(tInt, i, true)
	case token.FLOAT:
		f, err := strconv.ParseFloat(x.Value, 64)
		if err != nil || math.IsInf(f, 0) {
			in.fail(x, "the number "+x.Value+" is too large")
		}
		return fv(f, true)
	}
	in.fail(x, "only whole numbers and decimals are supported here")
	return value{}
}

func (in *interp) unaryExpr(x *ast.UnaryExpr) value {
	v := in.eval(x.X)
	switch x.Op {
	case token.SUB:
		if v.t.k == kInt {
			v.i = wrap(v.t, -v.i)
			return v
		}
		if v.t.k == kFloat {
			v.f = -v.f
			return v
		}
	case token.ADD:
		if v.t.k != kBool {
			return v
		}
	case token.NOT:
		if v.t.k == kBool {
			v.b = !v.b
			return v
		}
	case token.XOR:
		if v.t.k == kInt {
			v.i = wrap(v.t, ^v.i)
			return v
		}
	}
	in.fail(x, "the operator "+x.Op.String()+" can’t be used on a "+v.t.name)
	return value{}
}

func (in *interp) binaryExpr(x *ast.BinaryExpr) value {
	if x.Op == token.LAND || x.Op == token.LOR {
		l := in.eval(x.X)
		in.needBool(l, x.X)
		if (x.Op == token.LAND && !l.b) || (x.Op == token.LOR && l.b) {
			return bv(l.b)
		}
		r := in.eval(x.Y)
		in.needBool(r, x.Y)
		return bv(r.b)
	}
	a := in.eval(x.X)
	b := in.eval(x.Y)
	return in.binop(x, x.Op, a, b)
}

func isMath(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "math"
}

func (in *interp) selector(x *ast.SelectorExpr) value {
	if isMath(x.X) {
		switch x.Sel.Name {
		case "Pi":
			return fv(math.Pi, true)
		case "E":
			return fv(math.E, true)
		case "MaxInt64", "MaxInt":
			return iv(tInt, math.MaxInt64, true)
		case "MinInt64", "MinInt":
			return iv(tInt, math.MinInt64, true)
		case "MaxInt32":
			return iv(tInt, math.MaxInt32, true)
		case "MinInt32":
			return iv(tInt, math.MinInt32, true)
		case "MaxInt16":
			return iv(tInt, math.MaxInt16, true)
		case "MinInt16":
			return iv(tInt, math.MinInt16, true)
		case "MaxInt8":
			return iv(tInt, math.MaxInt8, true)
		case "MinInt8":
			return iv(tInt, math.MinInt8, true)
		case "MaxUint8":
			return iv(tInt, math.MaxUint8, true)
		case "MaxUint16":
			return iv(tInt, math.MaxUint16, true)
		case "MaxUint32":
			return iv(tInt, math.MaxUint32, true)
		}
	}
	in.fail(x, "this expression isn’t supported")
	return value{}
}

func (in *interp) callExpr(x *ast.CallExpr) value {
	switch f := x.Fun.(type) {
	case *ast.Ident:
		if t, ok := typeByName[f.Name]; ok {
			if len(x.Args) != 1 {
				in.fail(x, "a conversion to "+f.Name+" takes exactly one argument")
			}
			return in.conv(x, in.eval(x.Args[0]), t)
		}
		if f.Name == "min" || f.Name == "max" {
			return in.minmax(x, f.Name == "max")
		}
		in.fail(x, "the function “"+f.Name+"” isn’t supported. Available: number conversions (int, float64, …), min, max and math.Abs/Sqrt/Floor/Ceil/Trunc/Round/Pow/Mod/Max/Min.")
	case *ast.SelectorExpr:
		return in.mathCall(x, f)
	}
	in.fail(x, "this kind of call isn’t supported")
	return value{}
}

func (in *interp) minmax(x *ast.CallExpr, isMax bool) value {
	if len(x.Args) == 0 {
		in.fail(x, "min and max need at least one argument")
	}
	best := in.eval(x.Args[0])
	for _, a := range x.Args[1:] {
		next := in.eval(a)
		b2, n2 := in.unify(x, best, next)
		if (isMax && in.binop(x, token.GTR, n2, b2).b) || (!isMax && in.binop(x, token.LSS, n2, b2).b) {
			best = n2
		} else {
			best = b2
		}
	}
	return best
}

func (in *interp) mathCall(x *ast.CallExpr, sel *ast.SelectorExpr) value {
	if !isMath(sel.X) {
		in.fail(x, "only math.* functions are supported (and fmt.Print* as a statement)")
	}
	args := make([]float64, len(x.Args))
	for i, a := range x.Args {
		args[i] = in.convert(x, in.eval(a), tFloat).f
	}
	need := func(n int) {
		if len(args) != n {
			in.fail(x, "math."+sel.Sel.Name+" takes "+strconv.Itoa(n)+" argument(s)")
		}
	}
	switch sel.Sel.Name {
	case "Abs":
		need(1)
		return fv(math.Abs(args[0]), false)
	case "Sqrt":
		need(1)
		return fv(math.Sqrt(args[0]), false)
	case "Floor":
		need(1)
		return fv(math.Floor(args[0]), false)
	case "Ceil":
		need(1)
		return fv(math.Ceil(args[0]), false)
	case "Trunc":
		need(1)
		return fv(math.Trunc(args[0]), false)
	case "Round":
		need(1)
		return fv(math.Round(args[0]), false)
	case "Pow":
		need(2)
		return fv(math.Pow(args[0], args[1]), false)
	case "Mod":
		need(2)
		return fv(math.Mod(args[0], args[1]), false)
	case "Max":
		need(2)
		return fv(math.Max(args[0], args[1]), false)
	case "Min":
		need(2)
		return fv(math.Min(args[0], args[1]), false)
	}
	in.fail(x, fmt.Sprintf("math.%s isn’t supported", sel.Sel.Name))
	return value{}
}
