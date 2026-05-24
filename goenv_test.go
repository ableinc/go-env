package goenv

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// processConfig mirrors the real-world struct shape described in the user's project.
type processConfig struct {
	Port        int    `envconfig:"PORT"         default:"8080"`
	Environment string `envconfig:"ENV"          default:"development"`
	DatabaseURL string `envconfig:"DATABASE_URL" required:"true"`
	RedisURL    string `envconfig:"REDIS_URL"`
	JWTSecret   string `envconfig:"JWT_SECRET"   required:"true"`

	JWTAccessTTL  time.Duration `envconfig:"JWT_ACCESS_TTL"  default:"15m"`
	JWTRefreshTTL time.Duration `envconfig:"JWT_REFRESH_TTL" default:"168h"`

	Debug    bool    `envconfig:"DEBUG"     default:"false"`
	MaxConns int     `envconfig:"MAX_CONNS" default:"25"`
	Ratio    float64 `envconfig:"RATIO"     default:"0.75"`

	CORSOrigins string `envconfig:"CORS_ORIGINS" default:"http://localhost:3030"`

	Ignored string `ignored:"true"`
	SplitMe string `split_words:"true"` // → SPLIT_ME
	NoTag   string // no envconfig tag, no split_words → skipped
}

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

	LoadEnv(file)

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

	LoadEnv(file)

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

	LoadEnv(file)

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

	LoadEnv(file)

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

	LoadEnv(file)

	if os.Getenv("DUP_KEY") != "second" {
		t.Error("DUP_KEY should use the last value in the file")
	}
}

// --- Process tests ---

func TestProcessDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/testdb")
	t.Setenv("JWT_SECRET", "supersecret")

	var cfg processConfig
	if err := Process(&cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Port != 8080 {
		t.Errorf("Port: expected 8080, got %d", cfg.Port)
	}
	if cfg.Environment != "development" {
		t.Errorf("Environment: expected development, got %q", cfg.Environment)
	}
	if cfg.JWTAccessTTL != 15*time.Minute {
		t.Errorf("JWTAccessTTL: expected 15m, got %v", cfg.JWTAccessTTL)
	}
	if cfg.JWTRefreshTTL != 168*time.Hour {
		t.Errorf("JWTRefreshTTL: expected 168h, got %v", cfg.JWTRefreshTTL)
	}
	if cfg.Debug != false {
		t.Errorf("Debug: expected false, got %v", cfg.Debug)
	}
	if cfg.MaxConns != 25 {
		t.Errorf("MaxConns: expected 25, got %d", cfg.MaxConns)
	}
	if cfg.Ratio != 0.75 {
		t.Errorf("Ratio: expected 0.75, got %f", cfg.Ratio)
	}
	if cfg.CORSOrigins != "http://localhost:3030" {
		t.Errorf("CORSOrigins: expected http://localhost:3030, got %q", cfg.CORSOrigins)
	}
}

func TestProcessEnvOverridesDefault(t *testing.T) {
	t.Setenv("PORT", "9090")
	t.Setenv("ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://prod-host/db")
	t.Setenv("JWT_SECRET", "prod-secret")
	t.Setenv("JWT_ACCESS_TTL", "30m")
	t.Setenv("DEBUG", "true")
	t.Setenv("MAX_CONNS", "100")

	var cfg processConfig
	if err := Process(&cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Port != 9090 {
		t.Errorf("Port: expected 9090, got %d", cfg.Port)
	}
	if cfg.Environment != "production" {
		t.Errorf("Environment: expected production, got %q", cfg.Environment)
	}
	if cfg.JWTAccessTTL != 30*time.Minute {
		t.Errorf("JWTAccessTTL: expected 30m, got %v", cfg.JWTAccessTTL)
	}
	if cfg.Debug != true {
		t.Errorf("Debug: expected true, got %v", cfg.Debug)
	}
	if cfg.MaxConns != 100 {
		t.Errorf("MaxConns: expected 100, got %d", cfg.MaxConns)
	}
}

func TestProcessRequiredMissing(t *testing.T) {
	// DATABASE_URL and JWT_SECRET are both required; set neither.
	var cfg processConfig
	err := Process(&cfg)
	if err == nil {
		t.Fatal("expected error for missing required field, got nil")
	}
}

func TestProcessRequiredPresent(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/testdb")
	t.Setenv("JWT_SECRET", "mysecret")

	var cfg processConfig
	if err := Process(&cfg); err != nil {
		t.Fatalf("unexpected error when required fields are set: %v", err)
	}
	if cfg.DatabaseURL != "postgres://localhost/testdb" {
		t.Errorf("DatabaseURL: expected postgres://localhost/testdb, got %q", cfg.DatabaseURL)
	}
	if cfg.JWTSecret != "mysecret" {
		t.Errorf("JWTSecret: expected mysecret, got %q", cfg.JWTSecret)
	}
}

func TestProcessDuration(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/testdb")
	t.Setenv("JWT_SECRET", "s")
	t.Setenv("JWT_ACCESS_TTL", "5m30s")
	t.Setenv("JWT_REFRESH_TTL", "720h")

	var cfg processConfig
	if err := Process(&cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := 5*time.Minute + 30*time.Second
	if cfg.JWTAccessTTL != want {
		t.Errorf("JWTAccessTTL: expected %v, got %v", want, cfg.JWTAccessTTL)
	}
	if cfg.JWTRefreshTTL != 720*time.Hour {
		t.Errorf("JWTRefreshTTL: expected 720h, got %v", cfg.JWTRefreshTTL)
	}
}

func TestProcessInvalidDuration(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/testdb")
	t.Setenv("JWT_SECRET", "s")
	t.Setenv("JWT_ACCESS_TTL", "not-a-duration")

	var cfg processConfig
	if err := Process(&cfg); err == nil {
		t.Fatal("expected error for invalid duration, got nil")
	}
}

func TestProcessSplitWords(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/testdb")
	t.Setenv("JWT_SECRET", "s")
	t.Setenv("SPLIT_ME", "hello")

	var cfg processConfig
	if err := Process(&cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SplitMe != "hello" {
		t.Errorf("SplitMe: expected hello, got %q", cfg.SplitMe)
	}
}

func TestProcessIgnoredField(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/testdb")
	t.Setenv("JWT_SECRET", "s")
	t.Setenv("IGNORED", "should-be-ignored")

	var cfg processConfig
	if err := Process(&cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Ignored != "" {
		t.Errorf("Ignored field should not be populated, got %q", cfg.Ignored)
	}
}

func TestProcessUntaggedFieldSkipped(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/testdb")
	t.Setenv("JWT_SECRET", "s")
	t.Setenv("NO_TAG", "unexpected")

	var cfg processConfig
	if err := Process(&cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.NoTag != "" {
		t.Errorf("NoTag field should be skipped, got %q", cfg.NoTag)
	}
}

func TestProcessNilInput(t *testing.T) {
	if err := Process(nil); err == nil {
		t.Fatal("expected error for nil input, got nil")
	}
}

func TestProcessNonPointer(t *testing.T) {
	cfg := processConfig{}
	if err := Process(cfg); err == nil {
		t.Fatal("expected error for non-pointer input, got nil")
	}
}
