# Go env

A simple library to load environment variables from a file into your program at runtime.

## File format

```bash
# a comment
KEY=value
export KEY=value        # the `export` prefix used by shell-sourced files is ignored
SPACED = value          # whitespace around '=' is trimmed
DOUBLE="a \"quoted\" value"
SINGLE='no \n escapes'
```

If the same key appears multiple times, the last occurrence wins. Comment and quote
handling follows the shell:

| Line | Resulting value |
|------|-----------------|
| `URL=http://x.com/a # note` | `http://x.com/a` — in an unquoted value, a `" #"` or `"\t#"` comment is stripped |
| `HASH=#literal` | `#literal` — a `#` with no leading whitespace is part of the value |
| `EMPTY= # commented out` | *(empty)* — the leading space makes the `#` start a comment |
| `DOUBLE="a \"quoted\" value"` | `a "quoted" value` — double quotes honor `\"`, `\\`, `\n`, `\r`, `\t` and `\'` |
| `SINGLE='no \n escapes'` | `no \n escapes` — single quotes are literal; a backslash cannot escape the closing quote |
| `PARTIAL="no closing quote` | `no closing quote` — an unterminated quoted value runs to the end of the line |

An unrecognized escape such as `\q` is kept verbatim, backslash included.

## Install

```bash
go get github.com/ableinc/go-env
```

## API

### `LoadEnv(override bool, filepath ...string) error`

Loads environment variables from a `.env` file into the process environment. Defaults to `.env` if no path is given.

`override` controls what happens to variables that are already present in the process environment:

| `override` | Behavior |
|-----------|----------|
| `false` | Existing environment variables are left untouched; the file only fills in keys that are not already set. A variable set to the empty string still counts as set. |
| `true` | Values from the file replace any existing environment variables. |

Pass `false` unless you specifically want the file to win — the real environment is normally where deployment-time configuration comes from.

```go
import goenv "github.com/ableinc/go-env"

goenv.LoadEnv(false)                   // loads .env, keeps existing env vars
goenv.LoadEnv(true, ".env.development") // loads .env.development, file wins
```

A missing default `.env` is not an error, since configuration may come entirely from the real environment. An explicitly provided path that cannot be read **is** returned as an error:

```go
if err := goenv.LoadEnv(false, ".env.development"); err != nil {
    log.Fatal(err) // e.g. typo in the filename
}
```

---

### `Process(s any, override bool, filepath ...string) error`

Loads a `.env` file (defaults to `.env`) and maps the resulting environment variables into a struct using struct tags. `override` and `filepath` behave exactly as in `LoadEnv`.

```go
type Config struct {
    Host         string        `envconfig:"DB_HOST"      required:"true"`
    Port         int           `envconfig:"DB_PORT"      default:"5432"`
    Password     string        `envconfig:"DB_PASS"`
    ReadTimeout  time.Duration `envconfig:"READ_TIMEOUT" default:"30s"`
    AppEnv       string        `split_words:"true"`  // maps to APP_ENV
    Internal     string        `ignored:"true"`
}

var cfg Config
if err := goenv.Process(&cfg, false); err != nil {
    log.Fatal(err)
}

// or with a custom file, letting the file override existing env vars:
if err := goenv.Process(&cfg, true, ".env.development"); err != nil {
    log.Fatal(err)
}
```

`s` must be a pointer to a struct. A pointer to an interface holding a struct *value* is rejected with an error, because reflection cannot write through it; wrap a pointer instead (`var target any = &cfg`).

#### Nested and embedded structs

Nested and embedded structs are traversed, so tagged fields inside them are populated
using their own keys. Keys are **not** prefixed by the enclosing field name — each tag
names the env var it reads, wherever the field sits.

```go
type DB struct {
    Host string `envconfig:"DB_HOST" default:"localhost"`
    Port int    `envconfig:"DB_PORT" default:"5432"`
}

type Config struct {
    Telemetry            // embedded: its tagged fields are populated too
    DB      DB           // named: reads DB_HOST / DB_PORT, not CONFIG_DB_HOST
    Auth    *AuthConfig  // a nil pointer is allocated, then traversed
    Cache   *DB `ignored:"true"` // skips the whole subtree; stays nil
}
```

A nil pointer to a struct is allocated before being traversed, so it is non-nil after
`Process` returns. A field whose type is already being traversed further up the chain is
left alone, so self-referential and mutually recursive structs terminate. Unexported
fields are skipped, since reflection cannot write to them.

#### Struct Tags

| Tag | Description |
|-----|-------------|
| `envconfig:"KEY"` | Env var name to look up. Required unless `split_words` is set. |
| `split_words:"true"` | Derives the env key from the field name (`CamelCase` → `UPPER_SNAKE_CASE`). Ignored when `envconfig` is also set. |
| `required:"true"` | Returns an error if the env var is unset and no default is provided. |
| `default:"value"` | Fallback value when the env var is unset or empty. |
| `ignored:"true"` | Skips the field entirely. |

Supported field types: `string`, `int`/`int8`/`int16`/`int32`/`int64`, `uint`/`uint8`/`uint16`/`uint32`/`uint64`, `float32`/`float64`, `bool`, `time.Duration`.

Values are parsed at the field's own bit size, so a value that does not fit — `PORT=99999` into an `int16`, or `-1` into a `uint` — returns an error rather than silently wrapping around. A field carrying an `envconfig` or `split_words` tag whose type is not in the list above also returns an error, instead of being left at its zero value. That includes struct types such as `time.Time`: a struct field is only traversed as a container when it carries no key of its own. Untagged non-struct fields are skipped.

---

## Run Tests

```bash
make test
```

## Build from Source

```bash
make build
```
