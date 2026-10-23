package engine

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"sort"
	"strconv"
	"strings"
)

type loopInfo struct {
	line    int
	cv      []string
	changed map[string]bool
	hasExit bool
	noCond  bool
	isRange bool
}

// Program is a parsed, validated Go program ready to run.
type Program struct {
	fset     *token.FileSet
	off      int
	file     *ast.File
	main     *ast.FuncDecl
	loops    []*loopInfo
	ids      map[ast.Node]int
	declared map[string]bool
}

func (p *Program) line(pos token.Pos) int {
	ln := p.fset.Position(pos).Line - p.off
	if ln < 1 {
		ln = 1
	}
	return ln
}

func (p *Program) fail(n ast.Node, msg string) *CompileError {
	return &CompileError{Line: p.line(n.Pos()), Msg: msg}
}

// Compile parses Go source with go/parser. It accepts a full program
// (package main + func main), a func main() on its own, or just statements,
// which are wrapped in func main automatically.
func Compile(src string) (prog *Program, err error) {
	defer func() {
		if x := recover(); x != nil {
			if ce, ok := x.(*CompileError); ok {
				prog = nil
				err = ce
				return
			}
			panic(x)
		}
	}()
	if strings.TrimSpace(src) == "" {
		panic(&CompileError{Msg: "Paste some Go code first."})
	}
	if len(src) > 4000 {
		panic(&CompileError{Msg: "That’s too long for this demo (4000 characters max)."})
	}
	src = strings.ReplaceAll(src, "\r", "")
	if strings.Count(src, "\n") >= 150 {
		panic(&CompileError{Msg: "Too many lines for this demo (150 max)."})
	}

	text, off := src, 0
	first := ""
	if f := strings.Fields(src); len(f) > 0 {
		first = f[0]
	}
	switch {
	case first == "package":
	case first == "import" || strings.Contains(src, "func main"):
		text, off = "package main\n"+src, 1
	default:
		text, off = "package main\nfunc main() {\n"+src+"\n}\n", 2
	}

	fset := token.NewFileSet()
	file, perr := parser.ParseFile(fset, "input.go", text, parser.SkipObjectResolution)
	if perr != nil {
		ln, msg := 1, perr.Error()
		if el, ok := perr.(scanner.ErrorList); ok && len(el) > 0 {
			ln = el[0].Pos.Line - off
			msg = el[0].Msg
		}
		if ln < 1 {
			ln = 1
		}
		panic(&CompileError{Line: ln, Msg: "Syntax error: " + msg})
	}

	p := &Program{fset: fset, off: off, file: file, ids: map[ast.Node]int{}, declared: map[string]bool{}}
	for _, imp := range file.Imports {
		path, _ := strconv.Unquote(imp.Path.Value)
		if path != "fmt" && path != "math" {
			panic(p.fail(imp, "import “"+path+"” isn’t supported. Only fmt and math are available."))
		}
	}
	for _, d := range file.Decls {
		switch x := d.(type) {
		case *ast.FuncDecl:
			if x.Recv != nil || x.Name.Name != "main" || x.Type.Params.NumFields() != 0 || x.Type.Results != nil {
				panic(p.fail(x, "Only func main() is supported. Put your loop inside main."))
			}
			if x.Body == nil || p.main != nil {
				panic(p.fail(x, "I couldn’t read this func main()."))
			}
			p.main = x
		case *ast.GenDecl:
			if x.Tok != token.IMPORT {
				p.validate(x)
			}
		default:
			panic(p.fail(d, "this declaration isn’t supported."))
		}
	}
	if p.main == nil {
		panic(&CompileError{Msg: "I couldn’t find func main()."})
	}
	p.validate(p.main.Body)
	p.analyse()
	if len(p.loops) == 0 {
		panic(&CompileError{Msg: "I couldn’t find a loop. Add a for loop, for example:  for x > 0 {"})
	}
	return p, nil
}

func isFmt(c *ast.CallExpr) bool {
	sel, ok := c.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == "fmt"
}

func unsupportedName(n ast.Node) string {
	switch n.(type) {
	case *ast.FuncLit:
		return "function literals"
	case *ast.GoStmt:
		return "goroutines (go)"
	case *ast.DeferStmt:
		return "defer"
	case *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.CaseClause:
		return "switch (use if / else if instead)"
	case *ast.SelectStmt, *ast.SendStmt, *ast.CommClause, *ast.ChanType:
		return "channels"
	case *ast.LabeledStmt:
		return "labels"
	case *ast.CompositeLit, *ast.ArrayType, *ast.MapType, *ast.StructType, *ast.IndexExpr, *ast.SliceExpr, *ast.KeyValueExpr:
		return "slices, maps, structs and indexing"
	case *ast.StarExpr:
		return "pointers"
	}
	return fmt.Sprintf("this kind of code (%T)", n)
}

var assignOps = map[token.Token]token.Token{
	token.ADD_ASSIGN: token.ADD, token.SUB_ASSIGN: token.SUB, token.MUL_ASSIGN: token.MUL,
	token.QUO_ASSIGN: token.QUO, token.REM_ASSIGN: token.REM, token.AND_ASSIGN: token.AND,
	token.OR_ASSIGN: token.OR, token.XOR_ASSIGN: token.XOR, token.SHL_ASSIGN: token.SHL,
	token.SHR_ASSIGN: token.SHR, token.AND_NOT_ASSIGN: token.AND_NOT,
}

func (p *Program) checkAssign(x *ast.AssignStmt) {
	for _, l := range x.Lhs {
		if _, ok := l.(*ast.Ident); !ok {
			panic(p.fail(x, "only simple variables can be assigned to (no slices, fields or pointers)."))
		}
	}
	if x.Tok == token.DEFINE || x.Tok == token.ASSIGN {
		if len(x.Lhs) != len(x.Rhs) {
			panic(p.fail(x, fmt.Sprintf("assignment mismatch: %d variables but %d values (calls that return several values aren’t supported).", len(x.Lhs), len(x.Rhs))))
		}
		return
	}
	if _, ok := assignOps[x.Tok]; !ok || len(x.Lhs) != 1 || len(x.Rhs) != 1 {
		panic(p.fail(x, "this assignment operator isn’t supported."))
	}
}

// validate walks the AST once and rejects anything outside the supported subset.
func (p *Program) validate(root ast.Node) {
	ast.Inspect(root, func(n ast.Node) bool {
		switch x := n.(type) {
		case nil:
			return true
		case *ast.BlockStmt, *ast.ExprStmt, *ast.IncDecStmt, *ast.IfStmt, *ast.ForStmt, *ast.EmptyStmt,
			*ast.ParenExpr, *ast.BinaryExpr, *ast.Ident, *ast.ValueSpec, *ast.DeclStmt:
			return true
		case *ast.ReturnStmt:
			if len(x.Results) > 0 {
				panic(p.fail(x, "return values aren’t supported here."))
			}
		case *ast.AssignStmt:
			p.checkAssign(x)
		case *ast.BranchStmt:
			if x.Label != nil || (x.Tok != token.BREAK && x.Tok != token.CONTINUE) {
				panic(p.fail(x, "only plain break and continue are supported (no labels, goto or fallthrough)."))
			}
		case *ast.RangeStmt:
			if x.Value != nil {
				panic(p.fail(x, "range with two variables isn’t supported. Use  for i := range n"))
			}
			if x.Key != nil {
				if _, ok := x.Key.(*ast.Ident); !ok || x.Tok == token.ASSIGN {
					panic(p.fail(x, "use  for i := range n  (a new variable) in a range loop."))
				}
			}
		case *ast.GenDecl:
			if x.Tok != token.VAR && x.Tok != token.CONST {
				panic(p.fail(x, "type declarations aren’t supported."))
			}
		case *ast.BasicLit:
			if x.Kind != token.INT && x.Kind != token.FLOAT {
				panic(p.fail(x, "strings, characters and imaginary numbers aren’t supported here (they’re only allowed inside fmt.Print calls)."))
			}
		case *ast.UnaryExpr:
			switch x.Op {
			case token.ADD, token.SUB, token.NOT, token.XOR:
			default:
				panic(p.fail(x, "the operator "+x.Op.String()+" isn’t supported."))
			}
		case *ast.CallExpr:
			if isFmt(x) {
				return false // fmt.Print* output is ignored
			}
		case *ast.SelectorExpr:
			if id, ok := x.X.(*ast.Ident); !ok || (id.Name != "math" && id.Name != "fmt") {
				panic(p.fail(x, "this expression isn’t supported. Only math.* and fmt.Print* are available."))
			}
		default:
			panic(p.fail(n, unsupportedName(n)+" isn’t supported in this tool. Use plain int, float64 and bool variables with for loops."))
		}
		return true
	})
}

func isLoop(n ast.Node) bool {
	switch n.(type) {
	case *ast.ForStmt, *ast.RangeStmt:
		return true
	}
	return false
}

func addUnique(out *[]string, s string) {
	for _, u := range *out {
		if u == s {
			return
		}
	}
	*out = append(*out, s)
}

// collect gathers the variable names used in an expression.
func collect(n ast.Node, out *[]string) {
	ast.Inspect(n, func(m ast.Node) bool {
		switch x := m.(type) {
		case *ast.Ident:
			addUnique(out, x.Name)
		case *ast.SelectorExpr:
			return false
		case *ast.CallExpr:
			for _, a := range x.Args {
				collect(a, out)
			}
			return false
		}
		return true
	})
}

// scan reports which variables the nodes assign to, and whether a return or a
// break for the loop being scanned exists.
func scan(changed map[string]bool, root ast.Node) (exit bool) {
	if root == nil {
		return false
	}
	var stack []ast.Node
	depth := 0
	ast.Inspect(root, func(m ast.Node) bool {
		if m == nil {
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if isLoop(top) && top != root {
				depth--
			}
			return true
		}
		stack = append(stack, m)
		if isLoop(m) && m != root {
			depth++
		}
		switch y := m.(type) {
		case *ast.AssignStmt:
			for _, l := range y.Lhs {
				if id, ok := l.(*ast.Ident); ok {
					changed[id.Name] = true
				}
			}
		case *ast.IncDecStmt:
			if id, ok := y.X.(*ast.Ident); ok {
				changed[id.Name] = true
			}
		case *ast.ReturnStmt:
			exit = true
		case *ast.BranchStmt:
			if y.Tok == token.BREAK && depth == 0 {
				exit = true
			}
		}
		return true
	})
	return exit
}

// analyse numbers the loops (source order) and records what each one touches.
func (p *Program) analyse() {
	ast.Inspect(p.file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			if x.Tok == token.DEFINE {
				for _, l := range x.Lhs {
					if id, ok := l.(*ast.Ident); ok {
						p.declared[id.Name] = true
					}
				}
			}
		case *ast.ValueSpec:
			for _, nm := range x.Names {
				p.declared[nm.Name] = true
			}
		case *ast.RangeStmt:
			if id, ok := x.Key.(*ast.Ident); ok {
				p.declared[id.Name] = true
			}
		}
		return true
	})
	ast.Inspect(p.main.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.ForStmt:
			info := &loopInfo{line: p.line(x.Pos()), changed: map[string]bool{}, noCond: x.Cond == nil}
			if x.Cond != nil {
				var names []string
				collect(x.Cond, &names)
				for _, nm := range names {
					if p.declared[nm] {
						info.cv = append(info.cv, nm)
					}
				}
			}
			info.hasExit = scan(info.changed, x.Body)
			if x.Post != nil {
				scan(info.changed, x.Post)
			}
			p.loops = append(p.loops, info)
			p.ids[x] = len(p.loops)
		case *ast.RangeStmt:
			info := &loopInfo{line: p.line(x.Pos()), changed: map[string]bool{}, isRange: true}
			info.hasExit = scan(info.changed, x.Body)
			p.loops = append(p.loops, info)
			p.ids[x] = len(p.loops)
		}
		return true
	})
}

// notes gives a quick static read of each loop's condition.
func (p *Program) notes() []string {
	var out []string
	for i, l := range p.loops {
		if i == 3 {
			break
		}
		why := " so it can’t become false on its own."
		if l.hasExit {
			why = " so it only ends through a break or a return."
		}
		stuck := len(l.cv) > 0
		for _, v := range l.cv {
			if l.changed[v] {
				stuck = false
			}
		}
		switch {
		case l.isRange:
			out = append(out, fmt.Sprintf("Line %d: this loop counts through a fixed range, so it always finishes.", l.line))
		case l.noCond && l.hasExit:
			out = append(out, fmt.Sprintf("Line %d: the loop has no condition, so it only ends through a break or a return.", l.line))
		case l.noCond:
			out = append(out, fmt.Sprintf("Line %d: the loop has no condition and no break or return, so it can never stop.", l.line))
		case len(l.cv) == 0:
			out = append(out, fmt.Sprintf("Line %d: the condition uses no variables, so it never changes,%s", l.line, why))
		case stuck:
			pron := "it"
			if len(l.cv) > 1 {
				pron = "them"
			}
			out = append(out, fmt.Sprintf("Line %d: the condition uses %s but the loop never changes %s,%s", l.line, strings.Join(l.cv, ", "), pron, why))
		default:
			var ch []string
			for nm := range l.changed {
				ch = append(ch, nm)
			}
			sort.Strings(ch)
			changed := "nothing"
			if len(ch) > 0 {
				changed = strings.Join(ch, ", ")
			}
			out = append(out, fmt.Sprintf("Line %d: the condition uses %s; the loop changes: %s.", l.line, strings.Join(l.cv, ", "), changed))
		}
	}
	return out
}
