package goenv

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

	if err := LoadEnv(false, file); err != nil {
		t.Fatalf("LoadEnv: unexpected error: %v", err)
	}

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

	if err := LoadEnv(false, file); err != nil {
		t.Fatalf("LoadEnv: unexpected error: %v", err)
	}

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

	if err := LoadEnv(false, file); err != nil {
		t.Fatalf("LoadEnv: unexpected error: %v", err)
	}

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

	if err := LoadEnv(false, file); err != nil {
		t.Fatalf("LoadEnv: unexpected error: %v", err)
	}

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

	if err := LoadEnv(false, file); err != nil {
		t.Fatalf("LoadEnv: unexpected error: %v", err)
	}

	if os.Getenv("DUP_KEY") != "second" {
		t.Error("DUP_KEY should use the last value in the file")
	}
}

// --- Process tests ---

func TestProcessDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/testdb")
	t.Setenv("JWT_SECRET", "supersecret")

	var cfg processConfig
	if err := Process(&cfg, false); err != nil {
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
	if err := Process(&cfg, false); err != nil {
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
	err := Process(&cfg, false)
	if err == nil {
		t.Fatal("expected error for missing required field, got nil")
	}
}

func TestProcessRequiredPresent(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/testdb")
	t.Setenv("JWT_SECRET", "mysecret")

	var cfg processConfig
	if err := Process(&cfg, false); err != nil {
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
	if err := Process(&cfg, false); err != nil {
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
	if err := Process(&cfg, false); err == nil {
		t.Fatal("expected error for invalid duration, got nil")
	}
}

func TestProcessSplitWords(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/testdb")
	t.Setenv("JWT_SECRET", "s")
	t.Setenv("SPLIT_ME", "hello")

	var cfg processConfig
	if err := Process(&cfg, false); err != nil {
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
	if err := Process(&cfg, false); err != nil {
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
	if err := Process(&cfg, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.NoTag != "" {
		t.Errorf("NoTag field should be skipped, got %q", cfg.NoTag)
	}
}

func TestProcessNilInput(t *testing.T) {
	if err := Process(nil, false); err == nil {
		t.Fatal("expected error for nil input, got nil")
	}
}

func TestProcessNonPointer(t *testing.T) {
	cfg := processConfig{}
	if err := Process(cfg, false); err == nil {
		t.Fatal("expected error for non-pointer input, got nil")
	}
}

func TestLoadEnvInlineCommentStripped(t *testing.T) {
	file := fixturePath(t, "inline-comment.env.test")
	keys := []string{"DATABASE_URL", "MIGRATIONS_DIR", "DOUBLE_QUOTED", "SINGLE_QUOTED", "NO_COMMENT", "URL_HASH"}
	for _, k := range keys {
		t.Cleanup(func() { _ = os.Unsetenv(k) })
	}

	if err := LoadEnv(false, file); err != nil {
		t.Fatalf("LoadEnv: unexpected error: %v", err)
	}

	tests := map[string]string{
		"DATABASE_URL":   "postgres://user:pass@localhost:5432/db?sslmode=disable",
		"MIGRATIONS_DIR": "./migrations",
		"DOUBLE_QUOTED":  "http://localhost:3030/payment/success",
		"SINGLE_QUOTED":  "http://localhost:3030/payment/cancel",
		"NO_COMMENT":     "plainvalue",
		"URL_HASH":       "http://example.com#fragment",
	}
	for key, expected := range tests {
		if got := os.Getenv(key); got != expected {
			t.Errorf("%s: expected %q, got %q", key, expected, got)
		}
	}
}

// --- override behavior ---

// unsetKeys registers cleanup for keys a fixture may introduce into the process env.
func unsetKeys(t *testing.T, keys ...string) {
	t.Helper()
	for _, k := range keys {
		t.Cleanup(func() { _ = os.Unsetenv(k) })
	}
}

func TestLoadEnvOverrideFalseKeepsExisting(t *testing.T) {
	file := fixturePath(t, "basic.env.test")
	unsetKeys(t, "DB_HOST", "DB_USER", "DB_PASS")
	t.Setenv("APP_ENV", "preset-by-caller")

	if err := LoadEnv(false, file); err != nil {
		t.Fatalf("LoadEnv: unexpected error: %v", err)
	}

	if got := os.Getenv("APP_ENV"); got != "preset-by-caller" {
		t.Errorf("APP_ENV: expected the existing value to survive, got %q", got)
	}
	// Keys absent from the environment are still populated from the file.
	if got := os.Getenv("DB_HOST"); got != "localhost" {
		t.Errorf("DB_HOST: expected localhost, got %q", got)
	}
}

func TestLoadEnvOverrideTrueReplacesExisting(t *testing.T) {
	file := fixturePath(t, "basic.env.test")
	unsetKeys(t, "DB_HOST", "DB_USER", "DB_PASS")
	t.Setenv("APP_ENV", "preset-by-caller")

	if err := LoadEnv(true, file); err != nil {
		t.Fatalf("LoadEnv: unexpected error: %v", err)
	}

	if got := os.Getenv("APP_ENV"); got != "production" {
		t.Errorf("APP_ENV: expected the file value to win, got %q", got)
	}
}

func TestLoadEnvOverrideFalseKeepsEmptyExisting(t *testing.T) {
	file := fixturePath(t, "basic.env.test")
	unsetKeys(t, "DB_HOST", "DB_USER", "DB_PASS")
	// An explicitly empty variable is still "set" and must not be replaced.
	t.Setenv("APP_ENV", "")

	if err := LoadEnv(false, file); err != nil {
		t.Fatalf("LoadEnv: unexpected error: %v", err)
	}

	if got := os.Getenv("APP_ENV"); got != "" {
		t.Errorf("APP_ENV: expected the empty existing value to survive, got %q", got)
	}
}

func TestProcessOverrideFalseKeepsExisting(t *testing.T) {
	file := fixturePath(t, "process-override.env.test")
	t.Setenv("DATABASE_URL", "postgres://env-host/db")
	t.Setenv("JWT_SECRET", "env-secret")
	unsetKeys(t, "PORT")

	var cfg processConfig
	if err := Process(&cfg, false, file); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.DatabaseURL != "postgres://env-host/db" {
		t.Errorf("DatabaseURL: expected the existing env value, got %q", cfg.DatabaseURL)
	}
	if cfg.Port != 7777 {
		t.Errorf("Port: expected 7777 from the file, got %d", cfg.Port)
	}
}

func TestProcessOverrideTrueReplacesExisting(t *testing.T) {
	file := fixturePath(t, "process-override.env.test")
	t.Setenv("DATABASE_URL", "postgres://env-host/db")
	t.Setenv("JWT_SECRET", "env-secret")
	unsetKeys(t, "PORT")

	var cfg processConfig
	if err := Process(&cfg, true, file); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.DatabaseURL != "postgres://file-host/db" {
		t.Errorf("DatabaseURL: expected the file value to win, got %q", cfg.DatabaseURL)
	}
}

// --- file read errors ---

func TestLoadEnvExplicitMissingFileErrors(t *testing.T) {
	err := LoadEnv(false, filepath.Join(t.TempDir(), "absent.env"))
	if err == nil {
		t.Fatal("expected an error for an explicitly provided missing file, got nil")
	}
}

func TestProcessExplicitMissingFileErrors(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/testdb")
	t.Setenv("JWT_SECRET", "s")

	var cfg processConfig
	if err := Process(&cfg, false, filepath.Join(t.TempDir(), "absent.env")); err == nil {
		t.Fatal("expected an error for an explicitly provided missing file, got nil")
	}
}

func TestLoadEnvMissingDefaultFileIsNotAnError(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	if err := LoadEnv(false); err != nil {
		t.Errorf("a missing default %s should not be an error, got %v", DefaultEnvFile, err)
	}
}

// --- numeric range handling ---

type rangeConfig struct {
	Small  int8    `envconfig:"RANGE_SMALL"`
	USmall uint8   `envconfig:"RANGE_USMALL"`
	F32    float32 `envconfig:"RANGE_F32"`
}

func TestProcessIntOverflowErrors(t *testing.T) {
	t.Setenv("RANGE_SMALL", "300") // int8 max is 127

	var cfg rangeConfig
	if err := Process(&cfg, false); err == nil {
		t.Fatalf("expected an error for an int8 out of range, got nil (Small=%d)", cfg.Small)
	}
}

func TestProcessUintOverflowErrors(t *testing.T) {
	t.Setenv("RANGE_USMALL", "300") // uint8 max is 255

	var cfg rangeConfig
	if err := Process(&cfg, false); err == nil {
		t.Fatalf("expected an error for a uint8 out of range, got nil (USmall=%d)", cfg.USmall)
	}
}

func TestProcessNegativeUintErrors(t *testing.T) {
	t.Setenv("RANGE_USMALL", "-1")

	var cfg rangeConfig
	if err := Process(&cfg, false); err == nil {
		t.Fatalf("expected an error for a negative uint, got nil (USmall=%d)", cfg.USmall)
	}
}

func TestProcessFloatOverflowErrors(t *testing.T) {
	t.Setenv("RANGE_F32", "1e40") // float32 max is ~3.4e38

	var cfg rangeConfig
	if err := Process(&cfg, false); err == nil {
		t.Fatalf("expected an error for a float32 out of range, got nil (F32=%v)", cfg.F32)
	}
}

func TestProcessInRangeNumericsStillWork(t *testing.T) {
	t.Setenv("RANGE_SMALL", "-42")
	t.Setenv("RANGE_USMALL", "200")
	t.Setenv("RANGE_F32", "1.5")

	var cfg rangeConfig
	if err := Process(&cfg, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Small != -42 {
		t.Errorf("Small: expected -42, got %d", cfg.Small)
	}
	if cfg.USmall != 200 {
		t.Errorf("USmall: expected 200, got %d", cfg.USmall)
	}
	if cfg.F32 != 1.5 {
		t.Errorf("F32: expected 1.5, got %v", cfg.F32)
	}
}

// --- unsupported field types ---

type unsupportedConfig struct {
	List []string `envconfig:"UNSUPPORTED_LIST" required:"true"`
}

func TestProcessUnsupportedTaggedFieldErrors(t *testing.T) {
	t.Setenv("UNSUPPORTED_LIST", "a,b,c")

	var cfg unsupportedConfig
	if err := Process(&cfg, false); err == nil {
		t.Fatal("expected an error for a tagged field of an unsupported type, got nil")
	}
}

type untaggedNestedConfig struct {
	Name   string `envconfig:"NESTED_NAME"`
	Nested struct {
		Inner string
	}
}

func TestProcessUntaggedNestedFieldsAreSkipped(t *testing.T) {
	t.Setenv("NESTED_NAME", "outer")

	var cfg untaggedNestedConfig
	if err := Process(&cfg, false); err != nil {
		t.Fatalf("an untagged field inside a nested struct should be skipped, got %v", err)
	}
	if cfg.Name != "outer" {
		t.Errorf("Name: expected outer, got %q", cfg.Name)
	}
}

// --- pointer / interface unwrapping ---

func TestProcessInterfaceHoldingStructErrors(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/testdb")
	t.Setenv("JWT_SECRET", "s")

	var target any = processConfig{}
	if err := Process(&target, false); err == nil {
		t.Fatal("expected an error for an unaddressable struct inside an interface, got nil")
	}
}

func TestProcessInterfaceHoldingPointerWorks(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/testdb")
	t.Setenv("JWT_SECRET", "s")

	cfg := &processConfig{}
	var target any = cfg
	if err := Process(&target, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DatabaseURL != "postgres://localhost/testdb" {
		t.Errorf("DatabaseURL: expected the struct to be populated, got %q", cfg.DatabaseURL)
	}
}

func TestProcessNilPointerInsideInterfaceErrors(t *testing.T) {
	var target any = (*processConfig)(nil)
	if err := Process(&target, false); err == nil {
		t.Fatal("expected an error for a nil pointer inside an interface, got nil")
	}
}

func TestProcessDoublePointerWorks(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/testdb")
	t.Setenv("JWT_SECRET", "s")

	cfg := &processConfig{}
	if err := Process(&cfg, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DatabaseURL != "postgres://localhost/testdb" {
		t.Errorf("DatabaseURL: expected the struct to be populated, got %q", cfg.DatabaseURL)
	}
}

// --- export prefix ---

func TestLoadEnvExportPrefix(t *testing.T) {
	file := fixturePath(t, "export-prefix.env.test")
	tests := map[string]string{
		"EXP_PLAIN":       "one",
		"EXP_TABBED":      "two",
		"EXP_PADDED":      "three",
		"EXP_INDENTED":    "four",
		"EXP_QUOTED":      "five # not a comment",
		"EXP_BARE":        "six",
		"exportEXP_GLUED": "seven", // "export" not followed by whitespace
		"export":          "eight", // a variable that happens to be named export
		"EXP_SPACED":      "nine",  // spaces around '=' are trimmed
	}
	for k := range tests {
		unsetKeys(t, k)
	}

	if err := LoadEnv(false, file); err != nil {
		t.Fatalf("LoadEnv: unexpected error: %v", err)
	}

	for key, expected := range tests {
		if got := os.Getenv(key); got != expected {
			t.Errorf("%s: expected %q, got %q", key, expected, got)
		}
	}
}

// --- quoting and escapes ---

func TestLoadEnvQuotingAndEscapes(t *testing.T) {
	file := fixturePath(t, "escapes.env.test")
	tests := map[string]string{
		"ESC_INNER_QUOTE": `say "hello" now`,
		"ESC_BACKSLASH":   `C:\path\to`,
		"ESC_NEWLINE":     "line1\nline2",
		"ESC_TAB":         "a\tb",
		"ESC_CARRIAGE":    "a\rb",
		"ESC_UNKNOWN":     `keep \q as-is`, // unrecognized escapes are kept verbatim
		"ESC_TRAILING":    `dangling "`,    // unterminated: runs to end of line
		"ESC_APOSTROPHE":  "it's",
		// Single quotes are literal, as in the shell: no escape processing, and a
		// backslash cannot escape the closing quote.
		"ESC_SINGLE_LITERAL": `no \n escapes here`,
		"ESC_SINGLE_APOS":    `it\`,
		"ESC_QUOTED_HASH":    "value # not a comment",
		"ESC_EMPTY_COMMENT":  "",         // `KEY= # comment` is an empty value
		"ESC_HASH_VALUE":     "#literal", // `KEY=#hash` keeps the '#'
		"ESC_UNTERMINATED":   "no closing quote",
	}
	for k := range tests {
		unsetKeys(t, k)
	}

	if err := LoadEnv(false, file); err != nil {
		t.Fatalf("LoadEnv: unexpected error: %v", err)
	}

	for key, expected := range tests {
		if got := os.Getenv(key); got != expected {
			t.Errorf("%s: expected %q, got %q", key, expected, got)
		}
	}
}

// --- nested and embedded structs ---

type dbSection struct {
	Host string `envconfig:"NEST_DB_HOST" default:"dbhost"`
	Port int    `envconfig:"NEST_DB_PORT" default:"5432"`
}

type authSection struct {
	Secret string `envconfig:"NEST_SECRET" required:"true"`
}

type nestedConfig struct {
	AppName string `envconfig:"NEST_APP"`
	DB      dbSection
	Auth    *authSection
	Skipped *dbSection `ignored:"true"`
}

func TestProcessNestedStructs(t *testing.T) {
	t.Setenv("NEST_APP", "billing")
	t.Setenv("NEST_DB_HOST", "db.internal")
	t.Setenv("NEST_SECRET", "s3cret")

	var cfg nestedConfig
	if err := Process(&cfg, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.AppName != "billing" {
		t.Errorf("AppName: expected billing, got %q", cfg.AppName)
	}
	if cfg.DB.Host != "db.internal" {
		t.Errorf("DB.Host: expected db.internal, got %q", cfg.DB.Host)
	}
	if cfg.DB.Port != 5432 {
		t.Errorf("DB.Port: expected the nested default 5432, got %d", cfg.DB.Port)
	}
	if cfg.Auth == nil {
		t.Fatal("Auth: expected the nil pointer to be allocated")
	}
	if cfg.Auth.Secret != "s3cret" {
		t.Errorf("Auth.Secret: expected s3cret, got %q", cfg.Auth.Secret)
	}
	if cfg.Skipped != nil {
		t.Errorf("Skipped: an ignored subtree should be left alone, got %+v", cfg.Skipped)
	}
}

func TestProcessNestedRequiredMissing(t *testing.T) {
	// NEST_SECRET is required by the nested authSection; set neither it nor a default.
	var cfg nestedConfig
	if err := Process(&cfg, false); err == nil {
		t.Fatal("expected an error for a required field inside a nested struct, got nil")
	}
}

type nestedErrConfig struct {
	DB dbSection
}

func TestProcessNestedErrorNamesFieldPath(t *testing.T) {
	t.Setenv("NEST_DB_PORT", "not-a-number")

	var cfg nestedErrConfig
	err := Process(&cfg, false)
	if err == nil {
		t.Fatal("expected an error for an invalid nested value, got nil")
	}
	if !strings.Contains(err.Error(), "field DB.Port") {
		t.Errorf("expected the error to name the nested field path, got %v", err)
	}
}

type embeddedUnexportedInner struct {
	Region string `envconfig:"NEST_REGION"`
}

// ExportedInner is embedded by value to cover the exported-embedded case.
type ExportedInner struct {
	Zone string `envconfig:"NEST_ZONE"`
}

type embeddedConfig struct {
	embeddedUnexportedInner
	ExportedInner
	Name string `envconfig:"NEST_NAME"`
}

func TestProcessEmbeddedStructs(t *testing.T) {
	t.Setenv("NEST_REGION", "us-east-1")
	t.Setenv("NEST_ZONE", "us-east-1a")
	t.Setenv("NEST_NAME", "svc")

	var cfg embeddedConfig
	if err := Process(&cfg, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The embedded field itself is unsettable because its type is unexported, but
	// its exported fields are settable through it.
	if cfg.Region != "us-east-1" {
		t.Errorf("Region: expected us-east-1, got %q", cfg.Region)
	}
	if cfg.Zone != "us-east-1a" {
		t.Errorf("Zone: expected us-east-1a, got %q", cfg.Zone)
	}
	if cfg.Name != "svc" {
		t.Errorf("Name: expected svc, got %q", cfg.Name)
	}
}

func TestProcessEmbeddedErrorOmitsPromotedPrefix(t *testing.T) {
	t.Setenv("NEST_DB_PORT", "not-a-number")

	type embeddedErrConfig struct {
		dbSection
	}
	var cfg embeddedErrConfig
	err := Process(&cfg, false)
	if err == nil {
		t.Fatal("expected an error for an invalid embedded value, got nil")
	}
	// Embedded fields are promoted, so the path reads as it would in Go source.
	if !strings.Contains(err.Error(), "field Port") {
		t.Errorf("expected the error to name the promoted field, got %v", err)
	}
}

type deepPointerConfig struct {
	Deep **dbSection
}

func TestProcessAllocatesPointerChain(t *testing.T) {
	t.Setenv("NEST_DB_HOST", "deep.internal")

	var cfg deepPointerConfig
	if err := Process(&cfg, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Deep == nil || *cfg.Deep == nil {
		t.Fatal("Deep: expected the pointer chain to be allocated")
	}
	if (*cfg.Deep).Host != "deep.internal" {
		t.Errorf("Deep.Host: expected deep.internal, got %q", (*cfg.Deep).Host)
	}
}

type recursiveConfig struct {
	Label string `envconfig:"NEST_LABEL"`
	Next  *recursiveConfig
}

func TestProcessSelfReferentialStructTerminates(t *testing.T) {
	t.Setenv("NEST_LABEL", "head")

	var cfg recursiveConfig
	if err := Process(&cfg, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Label != "head" {
		t.Errorf("Label: expected head, got %q", cfg.Label)
	}
	if cfg.Next != nil {
		t.Errorf("Next: a self-referential field should be left nil, got %+v", cfg.Next)
	}
}

type mutualA struct {
	AVal string `envconfig:"NEST_A"`
	B    *mutualB
}

type mutualB struct {
	BVal string `envconfig:"NEST_B"`
	A    *mutualA
}

func TestProcessMutuallyRecursiveStructsTerminate(t *testing.T) {
	t.Setenv("NEST_A", "a")
	t.Setenv("NEST_B", "b")

	var cfg mutualA
	if err := Process(&cfg, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AVal != "a" {
		t.Errorf("AVal: expected a, got %q", cfg.AVal)
	}
	if cfg.B == nil {
		t.Fatal("B: expected the nil pointer to be allocated")
	}
	if cfg.B.BVal != "b" {
		t.Errorf("B.BVal: expected b, got %q", cfg.B.BVal)
	}
	if cfg.B.A != nil {
		t.Errorf("B.A: cycling back to a type on the path should be left nil, got %+v", cfg.B.A)
	}
}

type taggedStructConfig struct {
	When time.Time `envconfig:"NEST_WHEN"`
}

func TestProcessTaggedStructFieldStillErrors(t *testing.T) {
	t.Setenv("NEST_WHEN", "2026-09-01T00:00:00Z")

	var cfg taggedStructConfig
	err := Process(&cfg, false)
	if err == nil {
		t.Fatal("expected an error for a struct field that names an env var, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported field type") {
		t.Errorf("expected an unsupported-type error, got %v", err)
	}
}

type unexportedNestedConfig struct {
	hidden dbSection
	Name   string `envconfig:"NEST_NAME"`
}

func TestProcessUnexportedNestedStructIsSkipped(t *testing.T) {
	t.Setenv("NEST_DB_HOST", "db.internal")
	t.Setenv("NEST_NAME", "svc")

	var cfg unexportedNestedConfig
	if err := Process(&cfg, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.hidden.Host != "" {
		t.Errorf("hidden.Host: an unexported nested struct is unwritable, got %q", cfg.hidden.Host)
	}
	if cfg.Name != "svc" {
		t.Errorf("Name: expected svc, got %q", cfg.Name)
	}
}
