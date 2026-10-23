package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func do(t *testing.T, method, path, body, ctype string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	rec := httptest.NewRecorder()
	NewHandler(Config{}).ServeHTTP(rec, req)
	return rec
}

func decodeMap(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("response isn’t JSON: %v\n%s", err, rec.Body.String())
	}
	return m
}

func TestHealth(t *testing.T) {
	rec := do(t, http.MethodGet, "/api/health", "", "")
	if rec.Code != http.StatusOK || decodeMap(t, rec)["status"] != "ok" {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}

func TestPrograms(t *testing.T) {
	rec := do(t, http.MethodGet, "/api/programs", "", "")
	var list []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || rec.Code != 200 || len(list) != 5 {
		t.Fatalf("got %d, %d items, err %v", rec.Code, len(list), err)
	}
}

func TestSimulate(t *testing.T) {
	rec := do(t, http.MethodPost, "/api/simulate", `{"program":"collatz","start":27,"limit":200}`, "application/json")
	m := decodeMap(t, rec)
	if rec.Code != 200 || m["verdict"] != "halts" || m["steps"] != float64(111) {
		t.Fatalf("got %d %v", rec.Code, m)
	}
	if rec := do(t, http.MethodPost, "/api/simulate", `{"program":"nope","start":1,"limit":1}`, "application/json"); rec.Code != 404 {
		t.Fatalf("unknown program: got %d", rec.Code)
	}
}

func TestCheck(t *testing.T) {
	rec := do(t, http.MethodPost, "/api/check", `{"code":"a, b := 48, 18\nfor b != 0 {\n\ta, b = b, a%b\n}","limit":1000}`, "application/json")
	m := decodeMap(t, rec)
	if rec.Code != 200 || m["verdict"] != "halts" || m["final"] != "a=6, b=0" {
		t.Fatalf("got %d %v", rec.Code, m)
	}
	rec = do(t, http.MethodPost, "/api/check", `{"code":"for {\n}","limit":1000}`, "application/json")
	if m := decodeMap(t, rec); rec.Code != 200 || m["verdict"] != "loops" {
		t.Fatalf("got %d %v", rec.Code, m)
	}
	rec = do(t, http.MethodPost, "/api/check", `{"code":"x := 10\ny := 0\nfor x > 0 {\n\tx = x / y\n}","limit":1000}`, "application/json")
	if m := decodeMap(t, rec); rec.Code != 200 || m["verdict"] != "crash" {
		t.Fatalf("got %d %v", rec.Code, m)
	}
}

func TestCheckErrors(t *testing.T) {
	cases := []struct {
		name, method, path, body, ctype string
		want                           int
	}{
		{"syntax error", "POST", "/api/check", `{"code":"for x < 3 {\n"}`, "application/json", 422},
		{"unsupported code", "POST", "/api/check", `{"code":"s := \"hi\"\nfor {\n}"}`, "application/json", 422},
		{"undefined name at run time", "POST", "/api/check", `{"code":"for x > 0 {\n\tx--\n}"}`, "application/json", 422},
		{"empty code", "POST", "/api/check", `{"code":""}`, "application/json", 422},
		{"bad json", "POST", "/api/check", `{"code":`, "application/json", 400},
		{"unknown field", "POST", "/api/check", `{"code":"x=1","evil":true}`, "application/json", 400},
		{"two objects", "POST", "/api/check", `{"code":"x=1"}{"code":"x=2"}`, "application/json", 400},
		{"wrong content type", "POST", "/api/check", `{"code":"x=1"}`, "text/plain", 415},
		{"too large", "POST", "/api/check", `{"code":"` + strings.Repeat("a", 40000) + `"}`, "application/json", 413},
		{"unknown route", "GET", "/api/nope", "", "", 404},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, tc.method, tc.path, tc.body, tc.ctype)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tc.want, rec.Body.String())
			}
			if m := decodeMap(t, rec); m["error"] == nil {
				t.Fatalf("expected an error object, got %v", m)
			}
		})
	}
}
