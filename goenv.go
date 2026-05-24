package goenv

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"strconv"
	"sync"
	"time"
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
	deduped := pairs[write+1:]
	for i, j := 0, len(deduped)-1; i < j; i, j = i+1, j-1 {
		deduped[i], deduped[j] = deduped[j], deduped[i]
	}
	return deduped
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

// LoadEnv loads environment variables from a .env file using parallel goroutines.
func LoadEnv(filepath string, verbose bool) {
	start := time.Now()
	if verbose {
		fmt.Printf("Loading %s\n", filepath)
	}

	fileBytes, err := os.ReadFile(filepath)
	if err != nil {
		if verbose {
			fmt.Printf("Failed to open file: %v\n", err)
		}
		return
	}

	pairs := parseEnvFile(fileBytes)
	if len(pairs) == 0 {
		if verbose {
			fmt.Println("No valid environment variables found.")
		}
		return
	}
	pairs = dedupePairs(pairs)

	workerCount := 1
	if len(pairs) >= MAX_ENV_THRESHOLD {
		workerCount = min(max(runtime.NumCPU(), 1), len(pairs))
	}
	chunkSize := (len(pairs) + workerCount - 1) / workerCount

	var wg sync.WaitGroup
	var logCh chan string
	var logWg sync.WaitGroup
	if verbose {
		logCh = make(chan string, workerCount)
		logWg.Add(1)
		go func() {
			defer logWg.Done()
			for message := range logCh {
				fmt.Println(message)
			}
		}()
	}

	for i := 0; i < len(pairs); i += chunkSize {
		end := i + chunkSize
		end = min(end, len(pairs))
		wg.Add(1)
		go setEnvFromPairs(pairs[i:end], logCh, fmt.Sprintf("Worker %d", i/chunkSize+1), &wg)
	}

	wg.Wait()
	if logCh != nil {
		close(logCh)
		logWg.Wait()
	}

	if verbose {
		fmt.Printf("Completion time: %f seconds\n", time.Since(start).Seconds())
	}
}

// splitWords converts a CamelCase field name to UPPER_SNAKE_CASE for env var lookup.
// e.g. "ManualOverride" -> "MANUAL_OVERRIDE", "DBHost" -> "D_B_HOST"
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

// Process loads environment variables from an optional .env file into a struct.
// Supported struct tags:
//   - envconfig:"KEY"    – env var name to look up (required to process the field, unless split_words is set)
//   - split_words:"true" – derive the env key from the field name (CamelCase → UPPER_SNAKE_CASE); ignored when envconfig is also set
//   - required:"true"    – return an error if no value is available (env var and default are both empty)
//   - default:"value"    – fallback value when the env var is unset or empty
//   - ignored:"true"     – skip the field entirely
//
// If filepath is nil, no file is loaded and only existing environment variables are used.
func Process(filepath *string, s *interface{}) error {
	if filepath != nil {
		LoadEnv(*filepath, false)
	}

	if s == nil {
		return fmt.Errorf("Process: nil struct pointer")
	}

	// *interface{} -> interface{} -> concrete struct (or pointer to struct)
	iface := reflect.ValueOf(s).Elem()
	if iface.Kind() != reflect.Interface {
		return fmt.Errorf("Process: expected interface, got %s", iface.Kind())
	}
	inner := iface.Elem()
	if inner.Kind() == reflect.Ptr {
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
