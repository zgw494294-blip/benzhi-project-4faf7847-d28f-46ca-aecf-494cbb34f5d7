package web

import (
	"net/http"
	"strconv"
	"strings"

	"heritage-tree-relocation-permit/internal/application"
	"heritage-tree-relocation-permit/internal/domain"
)

func (h *Handler) ListCasesHandler(w http.ResponseWriter, r *http.Request) {
	query, err := parseCaseQuery(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	page, err := h.service.QueryCases(r.Context(), query)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": page.Items, "nextCursor": page.NextCursor, "summary": page.Summary, "correlationId": correlationID(r.Context())})
}

func (h *Handler) CreateCaseHandler(w http.ResponseWriter, r *http.Request) {
	var command application.CreateCaseCommand
	if err := decodeJSON(w, r, &command); err != nil {
		h.writeError(w, r, err)
		return
	}
	created, err := h.service.CreateCase(r.Context(), command)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *Handler) GetCaseHandler(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.GetCaseView(r.Context(), r.PathValue("caseID"))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) CompareRevisionsHandler(w http.ResponseWriter, r *http.Request) {
	from, err := strconv.Atoi(r.URL.Query().Get("from"))
	if err != nil {
		h.writeError(w, r, &application.ValidationError{Message: "from 必须为方案修订序号"})
		return
	}
	to, err := strconv.Atoi(r.URL.Query().Get("to"))
	if err != nil {
		h.writeError(w, r, &application.ValidationError{Message: "to 必须为方案修订序号"})
		return
	}
	result, err := h.service.CompareRevisions(r.Context(), r.PathValue("caseID"), from, to)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func parseCaseQuery(r *http.Request) (application.CaseQuery, error) {
	values := r.URL.Query()
	query := application.CaseQuery{Keyword: values.Get("keyword"), Cursor: values.Get("cursor")}
	if query.Keyword == "" {
		query.Keyword = values.Get("q")
	}
	if text := values.Get("limit"); text != "" {
		limit, err := strconv.Atoi(text)
		if err != nil {
			return application.CaseQuery{}, &application.ValidationError{Message: "limit 必须为整数"}
		}
		query.Limit = limit
	}
	for _, value := range values["status"] {
		for _, part := range strings.Split(value, ",") {
			status := strings.TrimSpace(part)
			if status == "" {
				continue
			}
			if status == "pending" {
				query.Statuses = append(query.Statuses, domain.StatusCorrectionRequired, domain.StatusPrecheckReady)
				continue
			}
			query.Statuses = append(query.Statuses, domain.CaseStatus(status))
		}
	}
	return query, nil
}

func (h *Handler) RecordAssessmentsHandler(w http.ResponseWriter, r *http.Request) {
	var command application.AssessmentCommand
	if err := decodeJSON(w, r, &command); err != nil {
		h.writeError(w, r, err)
		return
	}
	item, err := h.service.RecordAssessments(r.Context(), r.PathValue("caseID"), command)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) AddRevisionHandler(w http.ResponseWriter, r *http.Request) {
	var command application.RevisionCommand
	if err := decodeJSON(w, r, &command); err != nil {
		h.writeError(w, r, err)
		return
	}
	item, err := h.service.AddRevision(r.Context(), r.PathValue("caseID"), command)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) TimelineHandler(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.Timeline(r.Context(), r.PathValue("caseID"))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "correlationId": correlationID(r.Context())})
}
