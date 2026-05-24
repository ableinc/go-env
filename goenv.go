package goenv

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"strconv"
	"sync"
	"unicode"
)

type envPair struct {
	key   string
	value string
}

const MAX_ENV_THRESHOLD = 50
const DEFAULT_ENV_FILE = ".env"

// parseEnvFile parses .env file contents into key/value pairs, ignoring blanks and comments.
func parseEnvFile(data []byte) []envPair {
	if len(data) == 0 {
		return nil
	}
	estimated := bytes.Count(data, []byte{'\n'}) + 1
	pairs := make([]envPair, 0, estimated)

	for start := 0; start < len(data); {
		end := bytes.IndexByte(data[start:], '\n')
		if end == -1 {
			end = len(data) - start
		}
		line := bytes.TrimSpace(data[start : start+end])
		if end == len(data)-start {
			start = len(data)
		} else {
			start += end + 1
		}

		if len(line) == 0 || line[0] == '#' {
			continue
		}
		eq := bytes.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		key := bytes.TrimSpace(line[:eq])
		if len(key) == 0 {
			continue
		}
		value := bytes.TrimSpace(line[eq+1:])
		value = bytes.Trim(value, `"`)
		pairs = append(pairs, envPair{key: string(key), value: string(value)})
	}

	return pairs
}

// dedupePairs keeps the last occurrence of each key while preserving order.
func dedupePairs(pairs []envPair) []envPair {
	if len(pairs) <= 1 {
		return pairs
	}
	seen := make(map[string]struct{}, len(pairs))
	write := len(pairs) - 1
	for i := len(pairs) - 1; i >= 0; i-- {
		if _, exists := seen[pairs[i].key]; exists {
			continue
		}
		seen[pairs[i].key] = struct{}{}
		pairs[write] = pairs[i]
		write--
	}
	return pairs[write+1:]
}

// setEnvFromPairs sets environment variables from key/value pairs.
func setEnvFromPairs(pairs []envPair, ch chan<- string, name string, wg *sync.WaitGroup) {
	defer wg.Done()
	for _, pair := range pairs {
		if err := os.Setenv(pair.key, pair.value); err != nil && ch != nil {
			ch <- fmt.Sprintf("%s: Error setting %s", name, pair.key)
		}
	}
	if ch != nil {
		ch <- fmt.Sprintf("%s Complete", name)
	}
}

// readFile reads the .env file contents, defaulting to DEFAULT_ENV_FILE if no path is given.
func readFile(filepath string) ([]byte, error) {
	if filepath == "" {
		return os.ReadFile(DEFAULT_ENV_FILE)
	}
	return os.ReadFile(filepath)
}

// splitWords converts a CamelCase field name to UPPER_SNAKE_CASE for env var lookup.
// e.g. "ManualOverride" -> "MANUAL_OVERRIDE", "DBHost" -> "DB_HOST"
func splitWords(name string) string {
	var buf bytes.Buffer
	runes := []rune(name)
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) {
			prev := runes[i-1]
			next := rune(0)
			if i+1 < len(runes) {
				next = runes[i+1]
			}
			// Insert underscore before a capital that follows a lowercase,
			// or before a capital that precedes a lowercase (e.g. "DBHost" → "DB_Host").
			if unicode.IsLower(prev) || (unicode.IsUpper(prev) && unicode.IsLower(next)) {
				buf.WriteByte('_')
			}
		}
		buf.WriteRune(unicode.ToUpper(r))
	}
	return buf.String()
}

// setField assigns a string env value to a struct field, converting to the field's type.
func setField(fv reflect.Value, val string) error {
	switch fv.Kind() {
	case reflect.String:
		fv.SetString(val)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if val == "" {
			return nil
		}
		n, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return err
		}
		fv.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if val == "" {
			return nil
		}
		n, err := strconv.ParseUint(val, 10, 64)
		if err != nil {
			return err
		}
		fv.SetUint(n)
	case reflect.Float32, reflect.Float64:
		if val == "" {
			return nil
		}
		n, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return err
		}
		fv.SetFloat(n)
	case reflect.Bool:
		if val == "" {
			return nil
		}
		b, err := strconv.ParseBool(val)
		if err != nil {
			return err
		}
		fv.SetBool(b)
	}
	return nil
}

// LoadEnv loads environment variables from a .env file using parallel goroutines.
// If no filepath is provided, it defaults to DEFAULT_ENV_FILE (".env").
func LoadEnv(filepath ...string) {
	file := ""
	if len(filepath) > 0 {
		file = filepath[0]
	}

	fileBytes, err := readFile(file)
	if err != nil {
		return
	}

	pairs := parseEnvFile(fileBytes)
	if len(pairs) == 0 {
		return
	}
	pairs = dedupePairs(pairs)

	workerCount := 1
	if len(pairs) >= MAX_ENV_THRESHOLD {
		workerCount = min(max(runtime.NumCPU(), 1), len(pairs))
	}
	chunkSize := (len(pairs) + workerCount - 1) / workerCount

	var wg sync.WaitGroup

	for i := 0; i < len(pairs); i += chunkSize {
		end := min(i+chunkSize, len(pairs))
		wg.Add(1)
		go setEnvFromPairs(pairs[i:end], nil, fmt.Sprintf("Worker %d", i/chunkSize+1), &wg)
	}

	wg.Wait()
}

// Process loads environment variables from an optional .env file into a struct.
// Supported struct tags:
//   - envconfig:"KEY"    – env var name to look up (required to process the field, unless split_words is set)
//   - split_words:"true" – derive the env key from the field name (CamelCase → UPPER_SNAKE_CASE); ignored when envconfig is also set
//   - required:"true"    – return an error if no value is available (env var and default are both empty)
//   - default:"value"    – fallback value when the env var is unset or empty
//   - ignored:"true"     – skip the field entirely
//
// If no filepath is provided, DEFAULT_ENV_FILE (".env") is loaded.
func Process(s any, filepath ...string) error {
	LoadEnv(filepath...)

	if s == nil {
		return fmt.Errorf("Process: nil struct pointer")
	}

	// *interface{} -> interface{} -> concrete struct (or pointer to struct)
	iface := reflect.ValueOf(s).Elem()
	if iface.Kind() != reflect.Interface {
		return fmt.Errorf("Process: expected interface, got %s", iface.Kind())
	}
	inner := iface.Elem()
	if inner.Kind() == reflect.Pointer {
		inner = inner.Elem()
	}
	if inner.Kind() != reflect.Struct {
		return fmt.Errorf("Process: expected struct, got %s", inner.Kind())
	}

	t := inner.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fv := inner.Field(i)

		if !fv.CanSet() || field.Tag.Get("ignored") == "true" {
			continue
		}

		envKey := field.Tag.Get("envconfig")
		if envKey == "" {
			if field.Tag.Get("split_words") == "true" {
				envKey = splitWords(field.Name)
			} else {
				continue
			}
		}

		envVal := os.Getenv(envKey)
		if envVal == "" {
			if def := field.Tag.Get("default"); def != "" {
				envVal = def
			} else if field.Tag.Get("required") == "true" {
				return fmt.Errorf("Process: required env var %q is not set", envKey)
			}
		}

		if err := setField(fv, envVal); err != nil {
			return fmt.Errorf("Process: field %s: %w", field.Name, err)
		}
	}

	return nil
}
