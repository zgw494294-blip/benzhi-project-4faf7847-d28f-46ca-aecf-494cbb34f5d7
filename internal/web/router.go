package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"heritage-tree-relocation-permit/internal/application"
)

type contextKey string

const correlationKey contextKey = "correlation-id"

type Handler struct {
	service *application.Service
	assets  http.Handler
}

func NewHandler(service *application.Service, assets http.Handler) http.Handler {
	handler := &Handler{service: service, assets: assets}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", handler.HealthHandler)
	mux.HandleFunc("GET /api/cases", handler.ListCasesHandler)
	mux.HandleFunc("POST /api/cases", handler.CreateCaseHandler)
	mux.HandleFunc("GET /api/cases/{caseID}", handler.GetCaseHandler)
	mux.HandleFunc("POST /api/cases/{caseID}/assessments", handler.RecordAssessmentsHandler)
	mux.HandleFunc("POST /api/cases/{caseID}/revisions", handler.AddRevisionHandler)
	mux.HandleFunc("GET /api/cases/{caseID}/revisions/diff", handler.CompareRevisionsHandler)
	mux.HandleFunc("POST /api/cases/{caseID}/reviews", handler.SubmitReviewHandler)
	mux.HandleFunc("POST /api/cases/{caseID}/findings/{findingID}/evidence", handler.SubmitEvidenceHandler)
	mux.HandleFunc("POST /api/cases/{caseID}/findings/{findingID}/verify", handler.VerifyFindingHandler)
	mux.HandleFunc("POST /api/cases/{caseID}/prechecks", handler.RecordPrecheckHandler)
	mux.HandleFunc("GET /api/cases/{caseID}/precheck-checklist", handler.GetChecklistHandler)
	mux.HandleFunc("GET /api/cases/{caseID}/permit-eligibility", handler.PermitEligibilityHandler)
	mux.HandleFunc("POST /api/cases/{caseID}/permit", handler.IssuePermitHandler)
	mux.HandleFunc("GET /api/cases/{caseID}/permit", handler.GetPermitHandler)
	mux.HandleFunc("GET /api/cases/{caseID}/timeline", handler.TimelineHandler)
	mux.Handle("GET /", assets)
	return handler.middleware(mux)
}

func (h *Handler) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		correlation := r.Header.Get("X-Correlation-ID")
		if correlation == "" || len(correlation) > 128 {
			correlation = newCorrelationID()
		}
		w.Header().Set("X-Correlation-ID", correlation)
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; connect-src 'self'")
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), correlationKey, correlation)))
	})
}

func newCorrelationID() string {
	data := make([]byte, 12)
	if _, err := rand.Read(data); err != nil {
		return time.Now().UTC().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(data)
}
func correlationID(ctx context.Context) string {
	value, _ := ctx.Value(correlationKey).(string)
	return value
}

func (h *Handler) HealthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "time": time.Now().UTC(), "correlationId": correlationID(r.Context())})
}
