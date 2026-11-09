package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/fortega2/system-design-lab/app/internal/store"
)

// maxItemBody caps the size of a POST /items body.
const maxItemBody = 1 << 20

// ItemStore is what the items handlers need from the persistence layer.
type ItemStore interface {
	Get(ctx context.Context, id int64) (store.Item, error)
	Create(ctx context.Context, name, value string) (store.Item, error)
}

type createItemRequest struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// handleGetItem serves GET /items/{id}.
func (s *Server) handleGetItem(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id must be a positive integer"})
		return
	}

	item, err := s.items.Get(r.Context(), id)
	if err != nil {
		s.writeStoreError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

// handleCreateItem serves POST /items.
func (s *Server) handleCreateItem(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxItemBody)

	var req createItemRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	item, err := s.items.Create(r.Context(), req.Name, req.Value)
	if err != nil {
		s.writeStoreError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

// writeStoreError maps a store error to an HTTP response. Technical details
// are logged here, once, and never sent to the client.
func (s *Server) writeStoreError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "item not found"})
	case r.Context().Err() != nil:
		// The client is gone: nothing to answer. The metrics record a 499.
	case errors.Is(err, context.DeadlineExceeded):
		s.logger.WarnContext(r.Context(), "database timeout", "error", err)
		writeJSON(w, http.StatusGatewayTimeout, map[string]string{"error": "database timeout"})
	default:
		s.logger.ErrorContext(r.Context(), "store operation failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}
}
