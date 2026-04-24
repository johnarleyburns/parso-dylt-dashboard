package scraper

import (
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
)

func TestExtractPrice_FirstSelector(t *testing.T) {
	html := `<html><body><table><tbody><tr><td class="price">85.42</td></tr></tbody></table></body></html>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	got, err := extractPrice(doc, []string{"td.price", "td.other"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 85.42 {
		t.Errorf("got %v, want 85.42", got)
	}
}

func TestExtractPrice_FallsBackToSecondSelector(t *testing.T) {
	html := `<html><body><span class="settlement-price">101.75</span></body></html>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	got, err := extractPrice(doc, []string{"td.price", ".settlement-price"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 101.75 {
		t.Errorf("got %v, want 101.75", got)
	}
}

func TestExtractPrice_SkipsZeroAndNegative(t *testing.T) {
	// <td> must be inside a <table> — Go's HTML parser strips bare <td> elements.
	html := `<html><body><table><tbody><tr><td class="price">0</td><td class="other">42.10</td></tr></tbody></table></body></html>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	got, err := extractPrice(doc, []string{"td.price", "td.other"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 42.10 {
		t.Errorf("got %v, want 42.10", got)
	}
}

func TestExtractPrice_NotFound(t *testing.T) {
	html := `<html><body><p>no price here</p></body></html>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	_, err := extractPrice(doc, []string{"td.price"})
	if err == nil {
		t.Error("expected error when no price found")
	}
}

func TestExtractPrice_WithCommas(t *testing.T) {
	html := `<html><body><table><tbody><tr><td class="price">1,234.56</td></tr></tbody></table></body></html>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	got, err := extractPrice(doc, []string{"td.price"})
	if err != nil {
		t.Fatal(err)
	}
	if got != 1234.56 {
		t.Errorf("got %v, want 1234.56", got)
	}
}

func TestFrontMonthLabel_Format(t *testing.T) {
	label := frontMonthLabel()
	// Must parse as a date
	_, err := time.Parse("2006-01-02", label)
	if err != nil {
		t.Errorf("frontMonthLabel %q not a valid date: %v", label, err)
	}
	if !strings.HasSuffix(label, "-01") {
		t.Errorf("frontMonthLabel %q should end with -01 (first of month)", label)
	}
}

func TestFrontMonthLabel_IsNextMonth(t *testing.T) {
	label := frontMonthLabel()
	t.Helper()
	parsed, _ := time.Parse("2006-01-02", label)
	now := time.Now().UTC()
	next := now.AddDate(0, 1, 0)
	if parsed.Year() != next.Year() || parsed.Month() != next.Month() {
		t.Errorf("frontMonthLabel %q should be next month (%d-%02d), got %d-%02d",
			label, next.Year(), next.Month(), parsed.Year(), parsed.Month())
	}
}
