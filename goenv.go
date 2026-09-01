package goenv

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"time"
	"unicode"
)

type envPair struct {
	key   string
	value string
}

// DefaultEnvFile is the file LoadEnv and Process read when no path is given.
const DefaultEnvFile = ".env"

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
		line = trimExport(line)
		eq := bytes.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		key := bytes.TrimSpace(line[:eq])
		if len(key) == 0 {
			continue
		}
		pairs = append(pairs, envPair{key: string(key), value: parseValue(line[eq+1:])})
	}

	return pairs
}

// exportKeyword is the optional prefix used by .env files meant to be `source`d.
var exportKeyword = []byte("export")

// trimExport strips a leading "export" keyword so lines written for the shell
// (`export KEY=value`) parse the same as bare `KEY=value` lines. The keyword must be
// followed by whitespace, so `exportKEY=1` and `export=1` remain ordinary assignments.
func trimExport(line []byte) []byte {
	if !bytes.HasPrefix(line, exportKeyword) {
		return line
	}
	rest := line[len(exportKeyword):]
	if len(rest) == 0 || (rest[0] != ' ' && rest[0] != '\t') {
		return line
	}
	return bytes.TrimSpace(rest)
}

// parseValue unquotes the right-hand side of an assignment and strips inline comments.
// raw is the text after '=', before trimming, since leading whitespace decides whether a
// '#' starts a comment.
//
// Double-quoted values honor backslash escapes; single-quoted values are taken
// literally, as in the shell. Unquoted values have a trailing " #" or "\t#" comment
// removed.
func parseValue(raw []byte) string {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 {
		return ""
	}
	switch value[0] {
	case '"':
		return unescape(scanQuoted(value[1:], '"', true))
	case '\'':
		return string(scanQuoted(value[1:], '\'', false))
	case '#':
		// `KEY= # comment` is an empty value; `KEY=#hash` keeps the '#'.
		if raw[0] == ' ' || raw[0] == '\t' {
			return ""
		}
	}
	// strip inline comments: ' #' or '\t#'
	if idx := bytes.Index(value, []byte(" #")); idx >= 0 {
		value = bytes.TrimSpace(value[:idx])
	} else if idx := bytes.Index(value, []byte("\t#")); idx >= 0 {
		value = bytes.TrimSpace(value[:idx])
	}
	return string(value)
}

// scanQuoted returns the contents of a quoted value, up to the closing quote. When
// escapes is true, a backslash-escaped quote does not close the value. An unterminated
// value runs to the end of the line.
func scanQuoted(body []byte, quote byte, escapes bool) []byte {
	for i := 0; i < len(body); i++ {
		if escapes && body[i] == '\\' {
			i++ // the next byte is escaped, so it cannot close the value
			continue
		}
		if body[i] == quote {
			return body[:i]
		}
	}
	return body
}

// unescape resolves the backslash escapes recognized inside double-quoted values.
// An unrecognized escape is kept verbatim, backslash included.
func unescape(body []byte) string {
	if bytes.IndexByte(body, '\\') == -1 {
		return string(body)
	}
	var buf bytes.Buffer
	buf.Grow(len(body))
	for i := 0; i < len(body); i++ {
		if body[i] != '\\' || i+1 >= len(body) {
			buf.WriteByte(body[i])
			continue
		}
		i++
		switch body[i] {
		case 'n':
			buf.WriteByte('\n')
		case 'r':
			buf.WriteByte('\r')
		case 't':
			buf.WriteByte('\t')
		case '\\', '"', '\'':
			buf.WriteByte(body[i])
		default:
			buf.WriteByte('\\')
			buf.WriteByte(body[i])
		}
	}
	return buf.String()
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

// readFile reads the .env file contents, defaulting to DefaultEnvFile if no path is given.
func readFile(filepath string) ([]byte, error) {
	if filepath == "" {
		return os.ReadFile(DefaultEnvFile)
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
// Numeric values are parsed at the field's own bit size, so a value too large for the
// field is reported as an error rather than silently wrapping or becoming ±Inf.
func setField(fv reflect.Value, val string) error {
	if fv.Type() == reflect.TypeFor[time.Duration]() {
		if val == "" {
			return nil
		}
		d, err := time.ParseDuration(val)
		if err != nil {
			return err
		}
		fv.SetInt(int64(d))
		return nil
	}
	switch fv.Kind() {
	case reflect.String:
		fv.SetString(val)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if val == "" {
			return nil
		}
		n, err := strconv.ParseInt(val, 10, fv.Type().Bits())
		if err != nil {
			return err
		}
		fv.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if val == "" {
			return nil
		}
		n, err := strconv.ParseUint(val, 10, fv.Type().Bits())
		if err != nil {
			return err
		}
		fv.SetUint(n)
	case reflect.Float32, reflect.Float64:
		if val == "" {
			return nil
		}
		n, err := strconv.ParseFloat(val, fv.Type().Bits())
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
	default:
		return fmt.Errorf("unsupported field type %s", fv.Type())
	}
	return nil
}

// LoadEnv loads environment variables from a .env file into the process environment.
// If no filepath is provided, it defaults to DefaultEnvFile (".env").
//
// When override is false, keys that already exist in the process environment are left
// untouched, so the real environment wins over the file. When override is true, values
// from the file replace any existing ones.
//
// A missing default ".env" is not an error, since configuration may come entirely from
// the real environment. An explicitly provided path that cannot be read is an error.
func LoadEnv(override bool, filepath ...string) error {
	file := ""
	if len(filepath) > 0 {
		file = filepath[0]
	}

	fileBytes, err := readFile(file)
	if err != nil {
		if file == "" && errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("LoadEnv: %w", err)
	}

	pairs := parseEnvFile(fileBytes)
	if len(pairs) == 0 {
		return nil
	}
	pairs = dedupePairs(pairs)

	for _, pair := range pairs {
		if !override {
			if _, exists := os.LookupEnv(pair.key); exists {
				continue
			}
		}
		if err := os.Setenv(pair.key, pair.value); err != nil {
			return fmt.Errorf("LoadEnv: set %s: %w", pair.key, err)
		}
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
// Nested and embedded structs are traversed, so tagged fields inside them are populated
// using their own keys; keys are not prefixed by the enclosing field name. A nil pointer
// to a struct is allocated before being traversed.
//
// When override is false, existing environment variables take precedence over the file;
// when true, the file replaces them. See LoadEnv.
//
// If no filepath is provided, DefaultEnvFile (".env") is loaded.
func Process(s any, override bool, filepath ...string) error {
	if s == nil {
		return fmt.Errorf("Process: nil struct pointer")
	}

	rv := reflect.ValueOf(s)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("Process: expected non-nil pointer to struct, got %s", rv.Kind())
	}
	// Unwrap any interface and pointer indirection to reach the struct.
	inner := rv.Elem()
	for inner.Kind() == reflect.Interface || inner.Kind() == reflect.Pointer {
		if inner.IsNil() {
			return fmt.Errorf("Process: expected non-nil pointer to struct, got nil %s", inner.Kind())
		}
		inner = inner.Elem()
	}
	if inner.Kind() != reflect.Struct {
		return fmt.Errorf("Process: expected struct, got %s", inner.Kind())
	}
	// A struct stored in an interface is only readable through reflection, so writes
	// would be silently dropped. Report that instead of returning a zeroed struct.
	if !inner.CanSet() {
		return fmt.Errorf("Process: cannot set fields on unaddressable %s; pass a pointer to a struct rather than a pointer to an interface holding one", inner.Type())
	}

	if err := LoadEnv(override, filepath...); err != nil {
		return err
	}

	return processStruct(inner, "", map[reflect.Type]bool{})
}

// processStruct populates the tagged fields of v, descending into nested and embedded
// structs. path is the dotted field prefix used in error messages, and seen holds the
// struct types on the current path so self-referential types terminate.
func processStruct(v reflect.Value, path string, seen map[reflect.Type]bool) error {
	t := v.Type()
	seen[t] = true
	defer delete(seen, t)

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fv := v.Field(i)

		if field.Tag.Get("ignored") == "true" {
			continue
		}

		envKey := field.Tag.Get("envconfig")
		if envKey == "" && field.Tag.Get("split_words") == "true" {
			envKey = splitWords(field.Name)
		}

		// An untagged struct field is a container rather than a value, so descend
		// into it. A struct field that does name an env var is treated as a value,
		// and reported by setField as an unsupported type.
		if envKey == "" {
			if nested, ok := structToTraverse(fv, seen); ok {
				inner := path
				if !field.Anonymous {
					// Embedded fields are promoted, so only named fields
					// contribute to the path used in error messages.
					inner = path + field.Name + "."
				}
				if err := processStruct(nested, inner, seen); err != nil {
					return err
				}
			}
			continue
		}

		// Unexported fields cannot be written through reflection. Note that this is
		// checked after the traversal above: an embedded field of an unexported type
		// is itself unsettable, yet its exported fields are settable through it.
		if !fv.CanSet() {
			continue
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
			return fmt.Errorf("Process: field %s: %w", path+field.Name, err)
		}
	}

	return nil
}

// structToTraverse reports whether fv is a struct, or a pointer chain ending in one,
// that Process should descend into, allocating nil pointers along the way. A type
// already on the current path is refused so self-referential structs terminate.
func structToTraverse(fv reflect.Value, seen map[reflect.Type]bool) (reflect.Value, bool) {
	target := fv.Type()
	for target.Kind() == reflect.Pointer {
		target = target.Elem()
	}
	if target.Kind() != reflect.Struct || seen[target] {
		return reflect.Value{}, false
	}
	// Allocate as needed on the way down; checking `seen` first means a
	// self-referential field is left nil rather than allocated and abandoned.
	for fv.Kind() == reflect.Pointer {
		if fv.IsNil() {
			if !fv.CanSet() {
				return reflect.Value{}, false
			}
			fv.Set(reflect.New(fv.Type().Elem()))
		}
		fv = fv.Elem()
	}
	return fv, true
}
