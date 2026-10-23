package engine

import (
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	cases := []struct {
		name  string
		src   string
		want  Verdict
		steps int // -1 = don't check
		final string
	}{
		{"countdown", "x := 10\nfor x > 0 {\n\tx--\n}", Halts, 10, "x=0"},
		{"gcd tuple swap", "a, b := 48, 18\nfor b != 0 {\n\ta, b = b, a%b\n}", Halts, -1, "a=6, b=0"},
		{"break", "x := 0\nfor {\n\tx++\n\tif x > 5 {\n\t\tbreak\n\t}\n}", Halts, 6, "x=6"},
		{"nested finite", "i := 0\nfor i < 3 {\n\tj := 0\n\tfor j < 3 {\n\t\tj++\n\t}\n\ti++\n}", Halts, -1, "i=3"},
		{"nested inner stuck", "i := 0\nfor i < 3 {\n\tj := 0\n\tfor j < 3 {\n\t}\n\ti++\n}", Loops, -1, ""},
		{"classic for", "sum := 0\nfor i := 0; i < 10; i++ {\n\tsum += i\n}", Halts, 10, "sum=45"},
		{"range over int", "total := 0\nfor i := range 5 {\n\ttotal += i\n}", Halts, 5, "total=10"},
		{"collatz", "n := 27\nfor n != 1 {\n\tif n%2 == 0 {\n\t\tn /= 2\n\t} else {\n\t\tn = 3*n + 1\n\t}\n}", Halts, 111, "n=1"},
		{"float halving", "x := 1.0\nfor x > 0.001 {\n\tx /= 2\n}", Halts, -1, ""},
		{"empty infinite loop", "for {\n}", Loops, -1, ""},
		{"continue forever", "x := 5\nfor x > 0 {\n\tcontinue\n}", Loops, -1, ""},
		{"count up (needs 2^63 steps to wrap)", "x := 5\nfor x > 0 {\n\tx++\n}", Unknown, -1, ""},
		{"int8 wraps and halts", "x := int8(100)\nfor x > 0 {\n\tx += 10\n}", Halts, 3, "x=-126"},
		{"uint8 wraps around", "var x uint8 = 250\nfor x != 5 {\n\tx++\n}", Halts, 11, "x=5"},
		{"integer divide by zero", "x := 10\ny := 0\nfor x > 0 {\n\tx = x / y\n}", Crash, -1, ""},
		{"min and max builtins", "x := max(3, 7)\nfor x > 0 {\n\tx -= 2\n}", Halts, 4, "x=-1"},
		{"math.Sqrt converges", "x := 100.0\nfor x > 1 {\n\tx = math.Sqrt(x)\n}", Halts, -1, ""},
		{"return ends the program", "x := 3\nfor {\n\tx--\n\tif x == 0 {\n\t\treturn\n\t}\n}", Halts, 3, "x=0"},
		{"shadowed variable", "x := 2\nfor x > 0 {\n\tx := 1\n\t_ = x\n\tx = 0\n}", Loops, -1, ""},
		{"typed var declaration", "var x int = 5\nfor x > 0 {\n\tx -= 2\n}", Halts, 3, "x=-1"},
		{"fmt output ignored", "n := 3\nfor n > 0 {\n\tfmt.Println(\"n is\", n)\n\tn--\n}", Halts, 3, "n=0"},
		{"full program", "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tx := 3\n\tfor x > 0 {\n\t\tx--\n\t}\n\tfmt.Println(x)\n}", Halts, 3, "x=0"},
		{"func main without package", "func main() {\n\tn := 5\n\tfor n > 0 {\n\t\tn--\n\t}\n}", Halts, 5, "n=0"},
		{"global variable", "package main\n\nvar limit = 4\n\nfunc main() {\n\tx := 0\n\tfor x < limit {\n\t\tx++\n\t}\n}", Halts, 4, "limit=4, x=4"},
		{"logical operators", "x := 0\nfor x < 10 && x != 5 {\n\tx++\n}", Halts, 5, "x=5"},
		{"shift", "x := 1\nfor x < 100 {\n\tx <<= 1\n}", Halts, 7, "x=128"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := Compile(tc.src)
			if err != nil {
				t.Fatalf("compile error: %v", err)
			}
			res, err := p.Run(1000)
			if err != nil {
				t.Fatalf("run error: %v", err)
			}
			if res.Verdict != tc.want {
				t.Fatalf("verdict = %s, want %s (result %+v)", res.Verdict, tc.want, res)
			}
			if tc.steps >= 0 && res.Steps != tc.steps {
				t.Errorf("steps = %d, want %d", res.Steps, tc.steps)
			}
			if tc.final != "" && res.Final != tc.final {
				t.Errorf("final = %q, want %q", res.Final, tc.final)
			}
		})
	}
}

func TestCrashMessage(t *testing.T) {
	p, err := Compile("x := 10\ny := 0\nfor x > 0 {\n\tx = x / y\n}")
	if err != nil {
		t.Fatal(err)
	}
	res, _ := p.Run(100)
	if res.Verdict != Crash || !strings.Contains(res.Message, "integer divide by zero") {
		t.Fatalf("got %+v", res)
	}
}

func TestCompileErrors(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"empty", "", "Paste some Go code"},
		{"spaces only", "   \n\n  ", "Paste some Go code"},
		{"syntax error", "for x < 3 {\n", "Syntax error"},
		{"goroutine", "go func() {}()\nfor {\n}", "goroutines"},
		{"slice", "x := []int{1}\nfor {\n}", "slices, maps"},
		{"string literal", "s := \"hi\"\nfor {\n}", "strings"},
		{"switch", "x := 1\nswitch x {\ncase 1:\n}\nfor {\n}", "switch"},
		{"no loop", "x := 1", "couldn’t find a loop"},
		{"bad import", "package main\nimport \"os\"\nfunc main() {\n\tfor {\n\t}\n}", "“os”"},
		{"other function", "package main\nfunc helper() {}\nfunc main() {\n\tfor {\n\t}\n}", "Only func main()"},
		{"labels", "x := 0\nouter:\nfor x < 3 {\n\tx++\n}", "labels"},
		{"pointer", "x := 1\np := &x\nfor {\n\t_ = p\n}", "isn’t supported"},
		{"too long", strings.Repeat("x := 1\n", 800), "too long"},
		{"too many lines", strings.Repeat("\n", 200) + "x := 1", "Too many lines"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Compile(tc.src)
			if err == nil {
				t.Fatalf("expected an error containing %q", tc.want)
			}
			if _, ok := err.(*CompileError); !ok {
				t.Fatalf("error type = %T, want *CompileError", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tc.want)
			}
		})
	}
}

// Name and type errors are found when the offending line runs.
func TestRunErrors(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"undefined variable", "for x > 0 {\n\tx--\n}", "undefined: x"},
		{"mismatched types", "x := 1\ny := 2.5\nfor x < 3 {\n\tx = x + y\n}", "mismatched types"},
		{"constant overflow", "var x int8 = 300\nfor {\n}", "overflows int8"},
		{"non-bool condition", "x := 1\nfor x {\n\tx--\n}", "must be true or false"},
		{"unsupported type", "var x uint64 = 1\nfor {\n}", "isn’t supported"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := Compile(tc.src)
			if err != nil {
				t.Fatalf("compile error: %v", err)
			}
			_, err = p.Run(100)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestErrorLineNumbers(t *testing.T) {
	_, err := Compile("x := 0\nfor x < 3 {\n\ts := \"a\"\n}")
	ce, ok := err.(*CompileError)
	if !ok || ce.Line != 3 {
		t.Fatalf("want a CompileError on line 3, got %#v", err)
	}
	p, _ := Compile("x := 0\nfor x < 3 {\n\tx = x + undefinedName\n}")
	_, err = p.Run(10)
	if ce, ok := err.(*CompileError); !ok || ce.Line != 3 {
		t.Fatalf("want a runtime error on line 3, got %#v", err)
	}
}

func TestStepLimitClamp(t *testing.T) {
	cases := []struct {
		in       float64
		want     int
		adjusted bool
	}{{0, DefaultLimit, true}, {-5, DefaultLimit, true}, {500, 500, false}, {999999, MaxLimit, true}}
	for _, c := range cases {
		got, adj := ClampLimit(c.in)
		if got != c.want || adj != c.adjusted {
			t.Errorf("ClampLimit(%v) = %d,%v want %d,%v", c.in, got, adj, c.want, c.adjusted)
		}
	}
}

func TestSimulate(t *testing.T) {
	r, err := Simulate("collatz", 27, 200)
	if err != nil || r.Verdict != Halts || r.Steps != 111 {
		t.Fatalf("collatz 27: %+v, %v", r, err)
	}
	r, _ = Simulate("collatz", 0, 200) // invalid start becomes 1
	if r.Verdict != Halts || r.Steps != 0 {
		t.Fatalf("collatz 0: %+v", r)
	}
	r, _ = Simulate("flip", 0, 100)
	if r.Verdict != Loops || r.FirstSeen != 0 || r.RepeatedAt != 2 {
		t.Fatalf("flip: %+v", r)
	}
	r, _ = Simulate("up", 0, 100)
	if r.Verdict != Unknown {
		t.Fatalf("up: %+v", r)
	}
	r, _ = Simulate("slow", 5000, 100)
	if r.Verdict != Unknown {
		t.Fatalf("slow with small limit: %+v", r)
	}
	r, _ = Simulate("slow", 5000, 5000)
	if r.Verdict != Halts || r.Steps != 5000 {
		t.Fatalf("slow with big limit: %+v", r)
	}
	if _, err := Simulate("nope", 1, 1); err != ErrUnknownProgram {
		t.Fatalf("want ErrUnknownProgram, got %v", err)
	}
}
