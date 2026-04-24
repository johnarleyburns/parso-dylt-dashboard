package scraper

import (
	"testing"
)

func TestParseNumeric_Valid(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		{"82.45", 82.45},
		{"1,234.56", 1234.56},
		{"  99.9  ", 99.9},
		{"0.72", 0.72},
	}
	for _, c := range cases {
		got := parseNumeric(c.in)
		if got != c.want {
			t.Errorf("parseNumeric(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestParseNumeric_Invalid(t *testing.T) {
	cases := []string{"", "n/a", "--", "CLOSE", "abc"}
	for _, s := range cases {
		got := parseNumeric(s)
		if got != 0 {
			t.Errorf("parseNumeric(%q) = %v, want 0", s, got)
		}
	}
}

func TestContainsAny_Match(t *testing.T) {
	if !containsAny("Crude Oil Settlement", []string{"Crude Oil", "Natural Gas"}) {
		t.Error("expected containsAny to return true")
	}
}

func TestContainsAny_NoMatch(t *testing.T) {
	if containsAny("Electricity Futures", []string{"Crude Oil", "Natural Gas"}) {
		t.Error("expected containsAny to return false")
	}
}

func TestContainsAny_EmptySubstrs(t *testing.T) {
	if containsAny("anything", nil) {
		t.Error("expected false for empty substrs")
	}
}
