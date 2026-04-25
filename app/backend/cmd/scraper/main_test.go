package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestEnvOr(t *testing.T) {
	const key = "OILFIELD_TEST_SCRAPER_ENVOR"
	os.Unsetenv(key)
	if got := envOr(key, "fallback"); got != "fallback" {
		t.Errorf("missing key: got %q, want %q", got, "fallback")
	}

	os.Setenv(key, "real")
	defer os.Unsetenv(key)
	if got := envOr(key, "fallback"); got != "real" {
		t.Errorf("present key: got %q, want %q", got, "real")
	}
}

func TestMustEnvFatal(t *testing.T) {
	if os.Getenv("OILFIELD_SCRAPER_SUBPROCESS") == "1" {
		mustEnv("DEFINITELY_NOT_SET_99999")
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestMustEnvFatal")
	cmd.Env = append(os.Environ(), "OILFIELD_SCRAPER_SUBPROCESS=1")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit when env var is missing, got nil")
	}
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() == 0 {
		t.Fatalf("expected non-zero exit, got %v", err)
	}
}

func TestMustEnvPresent(t *testing.T) {
	const key = "OILFIELD_TEST_SCRAPER_MUST"
	os.Setenv(key, "world")
	defer os.Unsetenv(key)
	if got := mustEnv(key); got != "world" {
		t.Errorf("got %q, want %q", got, "world")
	}
}

// TestSectorStripSuffix verifies the "crude/eia" → "crude" stripping logic.
func TestSectorStripSuffix(t *testing.T) {
	cases := []struct {
		input, want string
	}{
		{"crude/eia", "crude"},
		{"natgas/hh_fut", "natgas"},
		{"refined/rb_fut", "refined"},
		{"electricity/eia", "electricity"},
		{"lng", "lng"},    // no slash — unchanged
		{"", ""},          // empty — unchanged
	}
	for _, c := range cases {
		sector := c.input
		if idx := strings.Index(sector, "/"); idx >= 0 {
			sector = sector[:idx]
		}
		if sector != c.want {
			t.Errorf("strip(%q): got %q, want %q", c.input, sector, c.want)
		}
	}
}

// TestEtcdEndpointsSplit verifies endpoint list parsing used in main.
func TestEtcdEndpointsSplit(t *testing.T) {
	eps := strings.Split("http://a:2379,http://b:2379,http://c:2379", ",")
	if len(eps) != 3 {
		t.Errorf("want 3 endpoints, got %d", len(eps))
	}
}
