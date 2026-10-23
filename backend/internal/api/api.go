// Package api exposes the engine over plain net/http (Go 1.22 pattern routing).
package api

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"halting/internal/engine"
)

const maxBody = 32 << 10 // 32 KB

type errDetail struct {
	Message string `json:"message"`
	Line    int    `json:"line,omitempty"`
}

type errBody struct {
	Error errDetail `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("encode response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string, line int) {
	writeJSON(w, status, errBody{Error: errDetail{Message: msg, Line: line}})
}

// decode reads one JSON object from the request body with strict limits.
func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "Send the request as application/json.", 0)
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "That request is too large.", 0)
			return false
		}
		writeError(w, http.StatusBadRequest, "The request body isn’t valid JSON for this endpoint.", 0)
		return false
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		writeError(w, http.StatusBadRequest, "Send a single JSON object.", 0)
		return false
	}
	return true
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handlePrograms(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, engine.Presets())
}

type simulateReq struct {
	Program string  `json:"program"`
	Start   float64 `json:"start"`
	Limit   float64 `json:"limit"`
}

func handleSimulate(w http.ResponseWriter, r *http.Request) {
	var req simulateReq
	if !decode(w, r, &req) {
		return
	}
	res, err := engine.Simulate(req.Program, req.Start, req.Limit)
	if errors.Is(err, engine.ErrUnknownProgram) {
		writeError(w, http.StatusNotFound, "Unknown program. Use GET /api/programs for the list.", 0)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal server error.", 0)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type checkReq struct {
	Code  string  `json:"code"`
	Limit float64 `json:"limit"`
}

func handleCheck(w http.ResponseWriter, r *http.Request) {
	var req checkReq
	if !decode(w, r, &req) {
		return
	}
	prog, err := engine.Compile(req.Code)
	if err != nil {
		writeCodeError(w, err)
		return
	}
	res, err := prog.Run(req.Limit)
	if err != nil {
		writeCodeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// writeCodeError reports a problem in the user's Go code as 422 with a line number.
func writeCodeError(w http.ResponseWriter, err error) {
	line := 0
	var ce *engine.CompileError
	if errors.As(err, &ce) {
		line = ce.Line
	}
	writeError(w, http.StatusUnprocessableEntity, err.Error(), line)
}

func handleNotFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "Not found.", 0)
}
