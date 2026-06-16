package api

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"
)

func TestNullStringPtr_Valid(t *testing.T) {
	ns := sql.NullString{String: "hello", Valid: true}
	result := nullStringPtr(ns)
	if result == nil {
		t.Fatal("expected non-nil pointer for valid NullString")
	}
	if *result != "hello" {
		t.Fatalf("expected 'hello', got '%s'", *result)
	}
}

func TestNullStringPtr_Invalid(t *testing.T) {
	ns := sql.NullString{String: "", Valid: false}
	result := nullStringPtr(ns)
	if result != nil {
		t.Fatal("expected nil for invalid NullString")
	}
}

func TestNullInt64Ptr_Valid(t *testing.T) {
	ni := sql.NullInt64{Int64: 42, Valid: true}
	result := nullInt64Ptr(ni)
	if result == nil {
		t.Fatal("expected non-nil pointer for valid NullInt64")
	}
	if *result != 42 {
		t.Fatalf("expected 42, got %d", *result)
	}
}

func TestNullInt64Ptr_Invalid(t *testing.T) {
	ni := sql.NullInt64{Int64: 0, Valid: false}
	result := nullInt64Ptr(ni)
	if result != nil {
		t.Fatal("expected nil for invalid NullInt64")
	}
}

func TestNullTimePtr_Valid(t *testing.T) {
	ts := time.Date(2024, 6, 15, 12, 30, 0, 0, time.UTC)
	nt := sql.NullTime{Time: ts, Valid: true}
	result := nullTimePtr(nt)
	if result == nil {
		t.Fatal("expected non-nil pointer for valid NullTime")
	}
	expected := "2024-06-15T12:30:00Z"
	if *result != expected {
		t.Fatalf("expected '%s', got '%s'", expected, *result)
	}
}

func TestNullTimePtr_Invalid(t *testing.T) {
	nt := sql.NullTime{Time: time.Time{}, Valid: false}
	result := nullTimePtr(nt)
	if result != nil {
		t.Fatal("expected nil for invalid NullTime")
	}
}

func TestNullStringPtr_JSONNull(t *testing.T) {
	// Verify that nil pointer serializes as JSON null (not omitted)
	type TestStruct struct {
		Name *string `json:"name"`
	}
	ns := sql.NullString{Valid: false}
	s := TestStruct{Name: nullStringPtr(ns)}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	expected := `{"name":null}`
	if string(data) != expected {
		t.Fatalf("expected %s, got %s", expected, string(data))
	}
}

func TestNullInt64Ptr_JSONNull(t *testing.T) {
	// Verify that nil pointer serializes as JSON null (not omitted)
	type TestStruct struct {
		PlanID *int64 `json:"plan_id"`
	}
	ni := sql.NullInt64{Valid: false}
	s := TestStruct{PlanID: nullInt64Ptr(ni)}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	expected := `{"plan_id":null}`
	if string(data) != expected {
		t.Fatalf("expected %s, got %s", expected, string(data))
	}
}

func TestNullTimePtr_JSONNull(t *testing.T) {
	// Verify that nil pointer serializes as JSON null (not omitted)
	type TestStruct struct {
		DeletedAt *string `json:"deleted_at"`
	}
	nt := sql.NullTime{Valid: false}
	s := TestStruct{DeletedAt: nullTimePtr(nt)}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	expected := `{"deleted_at":null}`
	if string(data) != expected {
		t.Fatalf("expected %s, got %s", expected, string(data))
	}
}

func TestNullTimePtr_RFC3339Format(t *testing.T) {
	// Verify the time is formatted correctly with timezone
	ts := time.Date(2023, 1, 2, 15, 4, 5, 0, time.FixedZone("EST", -5*3600))
	nt := sql.NullTime{Time: ts, Valid: true}
	result := nullTimePtr(nt)
	if result == nil {
		t.Fatal("expected non-nil pointer")
	}
	// Should be valid RFC3339
	_, err := time.Parse(time.RFC3339, *result)
	if err != nil {
		t.Fatalf("result '%s' is not valid RFC3339: %v", *result, err)
	}
}
