package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJSON_Success(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]string{"name": "test"}

	JSON(w, http.StatusOK, data)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}

	var resp APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !resp.Success {
		t.Error("expected success to be true")
	}
	if resp.Error != nil {
		t.Error("expected error to be nil")
	}
	if resp.Timestamp == "" {
		t.Error("expected timestamp to be set")
	}
}

func TestJSON_Created(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]int{"id": 42}

	JSON(w, http.StatusCreated, data)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", w.Code)
	}

	var resp APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !resp.Success {
		t.Error("expected success to be true")
	}
}

func TestError_BadRequest(t *testing.T) {
	w := httptest.NewRecorder()

	Error(w, http.StatusBadRequest, "INVALID_INPUT", "missing field")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	var resp APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Success {
		t.Error("expected success to be false")
	}
	if resp.Error == nil {
		t.Fatal("expected error to be present")
	}
	if resp.Error.Code != "INVALID_INPUT" {
		t.Errorf("expected error code INVALID_INPUT, got %s", resp.Error.Code)
	}
	if resp.Error.Message != "missing field" {
		t.Errorf("expected message 'missing field', got %s", resp.Error.Message)
	}
}

func TestError_NotFound(t *testing.T) {
	w := httptest.NewRecorder()

	Error(w, http.StatusNotFound, "NOT_FOUND", "resource not found")

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestErrorWithDetails(t *testing.T) {
	w := httptest.NewRecorder()

	ErrorWithDetails(w, http.StatusInternalServerError, "DB_ERROR", "query failed", "connection timeout")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", w.Code)
	}

	var resp APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Success {
		t.Error("expected success to be false")
	}
	if resp.Error == nil {
		t.Fatal("expected error to be present")
	}
	if resp.Error.Code != "DB_ERROR" {
		t.Errorf("expected code DB_ERROR, got %s", resp.Error.Code)
	}
	if resp.Error.Details != "connection timeout" {
		t.Errorf("expected details 'connection timeout', got %s", resp.Error.Details)
	}
}

func TestJSON_NilData(t *testing.T) {
	w := httptest.NewRecorder()

	JSON(w, http.StatusOK, nil)

	var resp APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !resp.Success {
		t.Error("expected success to be true")
	}
}
