//go:build !lite

package api

import (
	"testing"
)

func TestBuildSegmentWhere_SingleCondition(t *testing.T) {
	rules := SegmentRules{
		Conditions: []SegmentCondition{
			{Field: "status", Operator: "eq", Value: "active"},
		},
	}

	where, params, err := buildSegmentWhere(rules)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if where != "c.status = ?" {
		t.Errorf("where = %q, want %q", where, "c.status = ?")
	}
	if len(params) != 1 || params[0] != "active" {
		t.Errorf("params = %v, want [active]", params)
	}
}

func TestBuildSegmentWhere_MultipleConditions(t *testing.T) {
	rules := SegmentRules{
		Conditions: []SegmentCondition{
			{Field: "status", Operator: "eq", Value: "active"},
			{Field: "plan_id", Operator: "gt", Value: "5"},
		},
	}

	where, params, err := buildSegmentWhere(rules)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "c.status = ? AND c.plan_id > ?"
	if where != want {
		t.Errorf("where = %q, want %q", where, want)
	}
	if len(params) != 2 {
		t.Fatalf("params length = %d, want 2", len(params))
	}
	if params[0] != "active" || params[1] != "5" {
		t.Errorf("params = %v, want [active 5]", params)
	}
}

func TestBuildSegmentWhere_AllOperators(t *testing.T) {
	tests := []struct {
		name   string
		op     string
		wantOp string
	}{
		{"eq", "eq", "="},
		{"neq", "neq", "!="},
		{"gt", "gt", ">"},
		{"gte", "gte", ">="},
		{"lt", "lt", "<"},
		{"lte", "lte", "<="},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules := SegmentRules{
				Conditions: []SegmentCondition{
					{Field: "status", Operator: tt.op, Value: "x"},
				},
			}
			where, _, err := buildSegmentWhere(rules)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			want := "c.status " + tt.wantOp + " ?"
			if where != want {
				t.Errorf("where = %q, want %q", where, want)
			}
		})
	}
}

func TestBuildSegmentWhere_InvalidField(t *testing.T) {
	rules := SegmentRules{
		Conditions: []SegmentCondition{
			{Field: "password", Operator: "eq", Value: "secret"},
		},
	}

	_, _, err := buildSegmentWhere(rules)
	if err == nil {
		t.Fatal("expected error for invalid field, got nil")
	}
}

func TestBuildSegmentWhere_InvalidOperator(t *testing.T) {
	rules := SegmentRules{
		Conditions: []SegmentCondition{
			{Field: "status", Operator: "like", Value: "%admin%"},
		},
	}

	_, _, err := buildSegmentWhere(rules)
	if err == nil {
		t.Fatal("expected error for invalid operator, got nil")
	}
}

func TestBuildSegmentWhere_EmptyConditions(t *testing.T) {
	rules := SegmentRules{
		Conditions: []SegmentCondition{},
	}

	where, params, err := buildSegmentWhere(rules)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if where != "1=1" {
		t.Errorf("where = %q, want %q", where, "1=1")
	}
	if len(params) != 0 {
		t.Errorf("params = %v, want empty", params)
	}
}

func TestBuildSegmentWhere_AllFields(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		wantCol string
	}{
		{"status field", "status", "c.status"},
		{"plan_id field", "plan_id", "c.plan_id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules := SegmentRules{
				Conditions: []SegmentCondition{
					{Field: tt.field, Operator: "eq", Value: "test"},
				},
			}
			where, _, err := buildSegmentWhere(rules)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			want := tt.wantCol + " = ?"
			if where != want {
				t.Errorf("where = %q, want %q", where, want)
			}
		})
	}
}
