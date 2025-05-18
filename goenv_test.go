package goenv

import (
	"os"
	"strings"
	"testing"
)

func writeTempEnvFile(t *testing.T, contents string) string {
	t.Helper()
	tmpFile, err := os.CreateTemp("", ".env.test")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	_, err = tmpFile.WriteString(contents)
	if err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tmpFile.Close()
	return tmpFile.Name()
}

func TestLoadEnvBasic(t *testing.T) {
	content := `
# This is a comment
APP_ENV=production
DB_HOST=localhost
DB_USER=root
DB_PASS="supersecret"
# Another comment
`
	file := writeTempEnvFile(t, content)
	defer os.Remove(file)

	LoadEnv(file, false)

	tests := map[string]string{
		"APP_ENV": "production",
		"DB_HOST": "localhost",
		"DB_USER": "root",
		"DB_PASS": "supersecret",
	}

	for key, expected := range tests {
		value := os.Getenv(key)
		if value != expected {
			t.Errorf("Expected %s = %s, got %s", key, expected, value)
		}
	}
}

func TestLoadEnvWithMoreThan20Lines(t *testing.T) {
	var builder strings.Builder
	for i := 0; i < 25; i++ {
		builder.WriteString("KEY")
		builder.WriteString(string('A' + rune(i)))
		builder.WriteString("=VALUE")
		builder.WriteString(string('A' + rune(i)))
		builder.WriteString("\n")
	}
	file := writeTempEnvFile(t, builder.String())
	defer os.Remove(file)

	LoadEnv(file, false)

	for i := 0; i < 25; i++ {
		key := "KEY" + string('A'+rune(i))
		expected := "VALUE" + string('A'+rune(i))
		value := os.Getenv(key)
		if value != expected {
			t.Errorf("Expected %s = %s, got %s", key, expected, value)
		}
	}
}

func TestLoadEnvIgnoresInvalidLines(t *testing.T) {
	content := `
VALID_KEY=valid_value
INVALIDLINE
# Comment
ANOTHER_VALID=42
`
	file := writeTempEnvFile(t, content)
	defer os.Remove(file)

	LoadEnv(file, false)

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
