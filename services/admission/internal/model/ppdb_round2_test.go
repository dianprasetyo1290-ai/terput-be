package model

import (
	"reflect"
	"testing"
)

func TestValidateFormField(t *testing.T) {
	cases := []struct {
		name string
		f    FormField
		ok   bool
	}{
		{"text ok", FormField{Key: "hobby", Label: "Hobi", Type: "text"}, true},
		{"select ok", FormField{Key: "blood", Label: "Golongan darah", Type: "select", Options: []string{"A", "B"}}, true},
		{"builtin text ok", FormField{Key: "nisn", Label: "NISN", Type: "text", Required: true}, true},
		{"builtin wrong type", FormField{Key: "nisn", Label: "NISN", Type: "number"}, false},
		{"bad key", FormField{Key: "Hobby!", Label: "Hobi", Type: "text"}, false},
		{"no label", FormField{Key: "hobby", Type: "text"}, false},
		{"select one option", FormField{Key: "blood", Label: "Gol", Type: "select", Options: []string{"A"}}, false},
		{"select dup option", FormField{Key: "blood", Label: "Gol", Type: "select", Options: []string{"A", "A"}}, false},
		{"options on text", FormField{Key: "hobby", Label: "Hobi", Type: "text", Options: []string{"x"}}, false},
		{"unknown type", FormField{Key: "hobby", Label: "Hobi", Type: "file"}, false},
	}
	for _, c := range cases {
		if err := ValidateFormField(c.f); (err == nil) != c.ok {
			t.Errorf("%s: err=%v", c.name, err)
		}
	}
}

func TestNormalizeCustomValues(t *testing.T) {
	fields := []FormField{
		{Key: "blood", Label: "Gol", Type: "select", Options: []string{"A", "B"}},
		{Key: "height", Label: "Tinggi", Type: "number"},
		{Key: "since", Label: "Sejak", Type: "date"},
		{Key: "kip", Label: "KIP", Type: "checkbox"},
		{Key: "nisn", Label: "NISN", Type: "text"},
	}
	cases := []struct {
		name string
		in   map[string]any
		want map[string]any
		ok   bool
	}{
		{"valid + unknown dropped", map[string]any{"blood": "A", "height": "120", "since": "2020-01-02", "kip": true, "x": "y", "nisn": "1"},
			map[string]any{"blood": "A", "height": 120.0, "since": "2020-01-02", "kip": true}, true},
		{"empty dropped", map[string]any{"blood": " ", "height": "", "kip": false}, map[string]any{}, true},
		{"bad option", map[string]any{"blood": "O"}, nil, false},
		{"bad number", map[string]any{"height": "tinggi"}, nil, false},
		{"bad date", map[string]any{"since": "02-01-2020"}, nil, false},
		{"bad bool", map[string]any{"kip": "ya"}, nil, false},
	}
	for _, c := range cases {
		got, err := NormalizeCustomValues(fields, c.in)
		if (err == nil) != c.ok {
			t.Errorf("%s: err=%v", c.name, err)
			continue
		}
		if c.ok && !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestMissingFormFields(t *testing.T) {
	fields := []FormField{
		{Key: "nisn", Label: "NISN", Type: "text", Required: true},
		{Key: "blood", Label: "Gol", Type: "select", Required: true},
		{Key: "kip", Label: "KIP", Type: "checkbox", Required: true},
		{Key: "hobby", Label: "Hobi", Type: "text"},
	}
	got := MissingFormFields(fields, map[string]string{"nisn": ""}, map[string]any{"kip": true})
	if !reflect.DeepEqual(got, []string{"NISN", "Gol"}) {
		t.Fatalf("got %v", got)
	}
	if got := MissingFormFields(fields, map[string]string{"nisn": "123"}, map[string]any{"blood": "A", "kip": true}); len(got) != 0 {
		t.Fatalf("expected none, got %v", got)
	}
}

func TestValidateWaveLevel(t *testing.T) {
	g := func(n int) *int { return &n }
	cases := []struct {
		level string
		grade *int
		ok    bool
	}{
		{"", nil, true},
		{"SMP", nil, true},
		{"SMP", g(7), true},
		{"SMP", g(10), false},
		{"SD", g(1), true},
		{"TK", g(0), true},
		{"SMK", g(10), true},
		{"PAUD", nil, false},
		{"", g(7), false},
	}
	for _, c := range cases {
		if err := ValidateWaveLevel(c.level, c.grade); (err == nil) != c.ok {
			t.Errorf("%q %v: err=%v", c.level, c.grade, err)
		}
	}
	if !SameLevel("SMP", "SMP") || SameLevel("SMP", "SMA") || !SameLevel("", "SMA") {
		t.Fatal("SameLevel")
	}
}

func TestRefundAmount(t *testing.T) {
	cases := []struct {
		paid int64
		bp   int
		fee  int64
		want int64
	}{
		{1_000_000, 10000, 0, 1_000_000},
		{1_000_000, 5000, 0, 500_000},
		{1_000_000, 5000, 100_000, 400_000},
		{1_000_000, 0, 0, 0},
		{50_000, 10000, 100_000, 0},
		{0, 10000, 0, 0},
		{1_000_000, 20000, 0, 1_000_000},
	}
	for _, c := range cases {
		if got := RefundAmount(c.paid, c.bp, c.fee); got != c.want {
			t.Errorf("RefundAmount(%d,%d,%d)=%d want %d", c.paid, c.bp, c.fee, got, c.want)
		}
	}
	if MaskAccountNumber("1234567890") != "******7890" || MaskAccountNumber("123") != "123" {
		t.Fatal("mask")
	}
}
