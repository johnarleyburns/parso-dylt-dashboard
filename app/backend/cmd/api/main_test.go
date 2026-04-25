package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestEnvOr(t *testing.T) {
	const key = "OILFIELD_TEST_ENVOR_KEY"
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

// TestMustEnvFatal verifies that mustEnv exits the process when the key is absent.
// Uses subprocess invocation so the Fatalf doesn't kill the test runner.
func TestMustEnvFatal(t *testing.T) {
	if os.Getenv("OILFIELD_TEST_SUBPROCESS") == "1" {
		mustEnv("DEFINITELY_NOT_SET_12345")
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestMustEnvFatal")
	cmd.Env = append(os.Environ(), "OILFIELD_TEST_SUBPROCESS=1")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit when env var is missing, got nil")
	}
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() == 0 {
		t.Fatalf("expected non-zero exit, got %v", err)
	}
}

func TestMustEnvPresent(t *testing.T) {
	const key = "OILFIELD_TEST_MUSTENV_KEY"
	os.Setenv(key, "hello")
	defer os.Unsetenv(key)
	if got := mustEnv(key); got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

// TestEtcdEndpointsSplit verifies the endpoint splitting logic used in main.
func TestEtcdEndpointsSplit(t *testing.T) {
	single := strings.Split("http://127.0.0.1:2379", ",")
	if len(single) != 1 || single[0] != "http://127.0.0.1:2379" {
		t.Errorf("single endpoint: %v", single)
	}

	multi := strings.Split("http://a:2379,http://b:2379", ",")
	if len(multi) != 2 {
		t.Errorf("multi endpoint count: got %d, want 2", len(multi))
	}
	if multi[0] != "http://a:2379" || multi[1] != "http://b:2379" {
		t.Errorf("multi endpoints: %v", multi)
	}
}
