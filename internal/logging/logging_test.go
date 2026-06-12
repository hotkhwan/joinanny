package logging

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewWritesJSONLogRecords(t *testing.T) {
	var out bytes.Buffer
	logger, err := New("debug", &out)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	logger.Debug("logger ready", "component", "test")

	got := out.String()
	if !strings.Contains(got, `"msg":"logger ready"`) {
		t.Fatalf("log output = %q, want message", got)
	}
	if !strings.Contains(got, `"component":"test"`) {
		t.Fatalf("log output = %q, want component attribute", got)
	}
}

func TestNewRejectsUnknownLevel(t *testing.T) {
	var out bytes.Buffer
	_, err := New("verbose", &out)
	if err == nil {
		t.Fatal("New returned nil error, want validation error")
	}
}
