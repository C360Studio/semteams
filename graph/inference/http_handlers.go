// Package inference provides structural anomaly detection for missing relationships.
package inference

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// HTTPHandler provides HTTP endpoints for human review of structural anomalies.
type HTTPHandler struct {
	storage Storage
	applier RelationshipApplier
	logger  *slog.Logger
}

// NewHTTPHandler creates a new HTTP handler for inference endpoints.
func NewHTTPHandler(storage Storage, applier RelationshipApplier, logger *slog.Logger) *HTTPHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &HTTPHandler{
		storage: storage,
		applier: applier,
		logger:  logger,
	}
}

// RegisterHTTPHandlers registers inference endpoints with the given mux.
func (h *HTTPHandler) RegisterHTTPHandlers(prefix string, mux *http.ServeMux) {
	// Ensure prefix ends with /
	if !strings.HasSuffix(prefix, "/") {
		prefix = prefix + "/"
	}

	mux.HandleFunc(prefix+"anomalies/pending", h.handleListPending)
	mux.HandleFunc(prefix+"anomalies/", h.handleAnomalyByID)
	mux.HandleFunc(prefix+"stats", h.handleStats)

	h.logger.Info("Inference HTTP handlers registered", "prefix", prefix)
}

// ReviewRequest represents a human review decision.
type ReviewRequest struct {
	Decision          string `json:"decision"` // "approved" or "rejected"
	Notes             string `json:"notes,omitempty"`
	OverridePredicate string `json:"override_predicate,omitempty"`
	ReviewedBy        string `json:"reviewed_by,omitempty"`
}

// StatsResponse contains inference statistics.
type StatsResponse struct {
	TotalDetected int `json:"total_detected"`
	PendingReview int `json:"pending_review"`
	LLMApproved   int `json:"llm_approved"`
	LLMRejected   int `json:"llm_rejected"`
	HumanReview   int `json:"human_review"`
	HumanApproved int `json:"human_approved"`
	HumanRejected int `json:"human_rejected"`
	Applied       int `json:"applied"`
}

// handleListPending returns anomalies awaiting human review.
// GET /inference/anomalies/pending
func (h *HTTPHandler) handleListPending(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	anomalies, err := h.storage.GetByStatus(ctx, StatusHumanReview)
	if err != nil {
		h.logger.Error("Failed to get pending anomalies", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Return empty array instead of null
	if anomalies == nil {
		anomalies = []*StructuralAnomaly{}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(anomalies); err != nil {
		h.logger.Error("Failed to encode pending anomalies", "error", err)
	}
}

// handleAnomalyByID handles GET and POST for individual anomalies.
// GET  /inference/anomalies/{id} - Get anomaly details
// POST /inference/anomalies/{id}/review - Submit review decision
func (h *HTTPHandler) handleAnomalyByID(w http.ResponseWriter, r *http.Request) {
	anomalyID, isReview := h.parseAnomalyPath(r.URL.Path)

	if anomalyID == "" {
		http.Error(w, "Invalid anomaly ID", http.StatusBadRequest)
		return
	}

	// Reject reserved paths that should be handled by other handlers
	if anomalyID == "pending" || anomalyID == "stats" {
		http.Error(w, "Invalid anomaly ID", http.StatusBadRequest)
		return
	}

	if isReview {
		h.handleReviewAnomaly(w, r, anomalyID)
	} else {
		h.handleGetAnomaly(w, r, anomalyID)
	}
}

// parseAnomalyPath extracts the anomaly ID and whether it's a review request from URL path.
// Returns (anomalyID, isReview). Empty anomalyID indicates invalid path.
func (h *HTTPHandler) parseAnomalyPath(urlPath string) (string, bool) {
	// Normalize path: remove leading/trailing slashes
	path := strings.Trim(urlPath, "/")
	parts := strings.Split(path, "/")

	// Find "anomalies" segment and extract ID
	for i, part := range parts {
		if part == "anomalies" && i+1 < len(parts) {
			anomalyID := parts[i+1]

			// Validate ID is not empty after trimming
			anomalyID = strings.TrimSpace(anomalyID)
			if anomalyID == "" {
				return "", false
			}

			// Check if this is a review request
			isReview := i+2 < len(parts) && parts[i+2] == "review"

			return anomalyID, isReview
		}
	}

	return "", false
}

// handleGetAnomaly returns a single anomaly by ID.
// GET /inference/anomalies/{id}
func (h *HTTPHandler) handleGetAnomaly(w http.ResponseWriter, r *http.Request, anomalyID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	anomaly, err := h.storage.Get(ctx, anomalyID)
	if err != nil {
		h.logger.Error("Failed to get anomaly", "id", anomalyID, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if anomaly == nil {
		http.Error(w, "Anomaly not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(anomaly); err != nil {
		h.logger.Error("Failed to encode anomaly", "id", anomalyID, "error", err)
	}
}

// handleReviewAnomaly processes a human review decision.
// POST /inference/anomalies/{id}/review
func (h *HTTPHandler) handleReviewAnomaly(w http.ResponseWriter, r *http.Request, anomalyID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	// Parse request body
	var req ReviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate decision
	if req.Decision != "approved" && req.Decision != "rejected" {
		http.Error(w, "Decision must be 'approved' or 'rejected'", http.StatusBadRequest)
		return
	}

	// Get anomaly
	anomaly, err := h.storage.Get(ctx, anomalyID)
	if err != nil {
		h.logger.Error("Failed to get anomaly for review", "id", anomalyID, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if anomaly == nil {
		http.Error(w, "Anomaly not found", http.StatusNotFound)
		return
	}

	// Check if anomaly is in reviewable state
	if anomaly.Status != StatusHumanReview && anomaly.Status != StatusPending {
		http.Error(w, "Anomaly is not in reviewable state", http.StatusConflict)
		return
	}

	// Process decision
	reviewedBy := req.ReviewedBy
	if reviewedBy == "" {
		reviewedBy = "human"
	}

	now := time.Now()
	anomaly.ReviewedAt = &now
	anomaly.ReviewedBy = reviewedBy
	anomaly.ReviewNotes = req.Notes

	if req.Decision == "approved" {
		// Apply predicate override if provided
		if req.OverridePredicate != "" && anomaly.Suggestion != nil {
			anomaly.Suggestion.Predicate = req.OverridePredicate
		}

		// Apply the relationship
		if anomaly.Suggestion != nil && h.applier != nil {
			if err := h.applier.ApplyRelationship(ctx, anomaly.Suggestion); err != nil {
				h.logger.Error("Failed to apply approved relationship",
					"id", anomalyID, "error", err)
				http.Error(w, "Failed to apply relationship", http.StatusInternalServerError)
				return
			}
			anomaly.Status = StatusApplied
		} else {
			anomaly.Status = StatusApproved
		}
	} else {
		anomaly.Status = StatusRejected
	}

	// Save updated anomaly
	if err := h.storage.Save(ctx, anomaly); err != nil {
		h.logger.Error("Failed to save reviewed anomaly", "id", anomalyID, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	h.logger.Info("Anomaly reviewed",
		"id", anomalyID,
		"decision", req.Decision,
		"reviewed_by", reviewedBy)

	// Return updated anomaly
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(anomaly); err != nil {
		h.logger.Error("Failed to encode reviewed anomaly", "id", anomalyID, "error", err)
	}
}

// handleStats returns inference statistics.
// GET /inference/stats
func (h *HTTPHandler) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	counts, err := h.storage.Count(ctx)
	if err != nil {
		h.logger.Error("Failed to get anomaly counts", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Calculate total detected (sum of all statuses)
	totalDetected := 0
	for _, count := range counts {
		totalDetected += count
	}

	stats := StatsResponse{
		TotalDetected: totalDetected,
		PendingReview: counts[StatusPending],
		LLMApproved:   counts[StatusLLMApproved],
		LLMRejected:   counts[StatusLLMRejected],
		HumanReview:   counts[StatusHumanReview],
		HumanApproved: counts[StatusApproved],
		HumanRejected: counts[StatusRejected],
		Applied:       counts[StatusApplied],
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		h.logger.Error("Failed to encode stats", "error", err)
	}
}
