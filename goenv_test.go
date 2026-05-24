package goenv

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func fixturePath(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("Failed to resolve fixture path")
	}
	return filepath.Join(filepath.Dir(file), "testdata", name)
}

func TestLoadEnvBasic(t *testing.T) {
	file := fixturePath(t, "basic.env.test")

	LoadEnv(file, false)

	tests := map[string]string{
		"APP_ENV": "production",
		"DB_HOST": "localhost",
		"DB_USER": "root",
		"DB_PASS": "supersecret",
	}

	for key, expected := range tests {
		t.Cleanup(func() { _ = os.Unsetenv(key) })
		value := os.Getenv(key)
		if value != expected {
			t.Errorf("Expected %s = %s, got %s", key, expected, value)
		}
	}
}

func TestLoadEnvUnder50File(t *testing.T) {
	file := fixturePath(t, "under-50.env.test")

	LoadEnv(file, false)

	tests := map[string]string{
		"UKEY01": "VALUE01",
		"UKEY05": "VALUE05",
		"UKEY10": "VALUE10",
	}
	for key, expected := range tests {
		t.Cleanup(func() { _ = os.Unsetenv(key) })
		if value := os.Getenv(key); value != expected {
			t.Errorf("Expected %s = %s, got %s", key, expected, value)
		}
	}
}

func TestLoadEnvOver50File(t *testing.T) {
	file := fixturePath(t, "over-50.env.test")

	LoadEnv(file, false)

	tests := map[string]string{
		"OKEY01": "VALUE01",
		"OKEY25": "VALUE25",
		"OKEY60": "VALUE60",
	}
	for key, expected := range tests {
		t.Cleanup(func() { _ = os.Unsetenv(key) })
		if value := os.Getenv(key); value != expected {
			t.Errorf("Expected %s = %s, got %s", key, expected, value)
		}
	}
}

func TestLoadEnvIgnoresInvalidLines(t *testing.T) {
	file := fixturePath(t, "invalid.env.test")

	LoadEnv(file, false)

	t.Cleanup(func() { _ = os.Unsetenv("VALID_KEY") })
	t.Cleanup(func() { _ = os.Unsetenv("ANOTHER_VALID") })
	if os.Getenv("VALID_KEY") != "valid_value" {
		t.Error("VALID_KEY should be set to valid_value")
	}
	if os.Getenv("ANOTHER_VALID") != "42" {
		t.Error("ANOTHER_VALID should be set to 42")
	}
	if os.Getenv("INVALIDLINE") != "" {
		t.Error("INVALIDLINE should not be set")
	}
}

func TestLoadEnvDuplicateKeysLastWins(t *testing.T) {
	file := fixturePath(t, "duplicate.env.test")
	defer os.Unsetenv("DUP_KEY")

	LoadEnv(file, false)

	if os.Getenv("DUP_KEY") != "second" {
		t.Error("DUP_KEY should use the last value in the file")
	}
}
