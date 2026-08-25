package web

import (
	"net/http"

	"heritage-tree-relocation-permit/internal/application"
)

func (h *Handler) SubmitReviewHandler(w http.ResponseWriter, r *http.Request) {
	var command application.ReviewCommand
	if err := decodeJSON(w, r, &command); err != nil {
		h.writeError(w, r, err)
		return
	}
	item, err := h.service.SubmitReview(r.Context(), r.PathValue("caseID"), command)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) SubmitEvidenceHandler(w http.ResponseWriter, r *http.Request) {
	var command application.EvidenceCommand
	if err := decodeJSON(w, r, &command); err != nil {
		h.writeError(w, r, err)
		return
	}
	item, err := h.service.SubmitEvidence(r.Context(), r.PathValue("caseID"), r.PathValue("findingID"), command)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) VerifyFindingHandler(w http.ResponseWriter, r *http.Request) {
	var command application.VerifyFindingCommand
	if err := decodeJSON(w, r, &command); err != nil {
		h.writeError(w, r, err)
		return
	}
	item, err := h.service.VerifyFinding(r.Context(), r.PathValue("caseID"), r.PathValue("findingID"), command)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) RecordPrecheckHandler(w http.ResponseWriter, r *http.Request) {
	var command application.PrecheckCommand
	if err := decodeJSON(w, r, &command); err != nil {
		h.writeError(w, r, err)
		return
	}
	item, err := h.service.RecordPrecheck(r.Context(), r.PathValue("caseID"), command)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) GetChecklistHandler(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.CurrentChecklist(r.Context(), r.PathValue("caseID"))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) PermitEligibilityHandler(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.PermitEligibility(r.Context(), r.PathValue("caseID"))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) IssuePermitHandler(w http.ResponseWriter, r *http.Request) {
	var command application.IssuePermitCommand
	if err := decodeJSON(w, r, &command); err != nil {
		h.writeError(w, r, err)
		return
	}
	item, err := h.service.IssuePermit(r.Context(), r.PathValue("caseID"), command)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h *Handler) GetPermitHandler(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.GetPermit(r.Context(), r.PathValue("caseID"))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}
