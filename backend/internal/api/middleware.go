package api

import (
	"log"
	"net/http"
	"runtime/debug"
	"time"
)

// Config holds optional settings for the HTTP layer.
type Config struct {
	// CORSOrigin, if set, is sent as Access-Control-Allow-Origin. Leave it empty
	// when the browser reaches this server through the Next.js /api rewrite.
	CORSOrigin string
}

// NewHandler builds the router and wraps it in middleware.
func NewHandler(cfg Config) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", handleHealth)
	mux.HandleFunc("GET /api/programs", handlePrograms)
	mux.HandleFunc("POST /api/simulate", handleSimulate)
	mux.HandleFunc("POST /api/check", handleCheck)
	mux.HandleFunc("/api/", handleNotFound)
	return chain(mux, recoverer, logger, securityHeaders, cors(cfg.CORSOrigin), timeout)
}

func chain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if x := recover(); x != nil {
				log.Printf("panic: %v\n%s", x, debug.Stack())
				writeError(w, http.StatusInternalServerError, "Internal server error.", 0)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, sw.status, time.Since(start).Round(time.Microsecond))
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func cors(origin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if origin == "" {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func timeout(next http.Handler) http.Handler {
	return http.TimeoutHandler(next, 5*time.Second, `{"error":{"message":"The request took too long."}}`)
}
