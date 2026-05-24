# Go env

A simple library to load environment variables from a file into your program at runtime. For large `.env` files (50+ variables), it splits the work across goroutines based on the number of CPUs. If the same key appears multiple times, the last occurrence wins.

## Install

```bash
go get github.com/ableinc/go-env
```

## API

### `LoadEnv(filepath ...string)`

Loads environment variables from a `.env` file into the process environment. Defaults to `.env` if no path is given.

```go
import goenv "github.com/ableinc/go-env"

goenv.LoadEnv()                   // loads .env
goenv.LoadEnv(".env.development") // loads .env.development
```

---

### `Process(s any, filepath ...string) error`

Loads a `.env` file (defaults to `.env`) and maps the resulting environment variables into a struct using struct tags.

```go
type Config struct {
    Host     string `envconfig:"DB_HOST" required:"true"`
    Port     int    `envconfig:"DB_PORT" default:"5432"`
    Password string `envconfig:"DB_PASS"`
    AppEnv   string `split_words:"true"`  // maps to APP_ENV
    Internal string `ignored:"true"`
}

var cfg Config
if err := goenv.Process(&cfg); err != nil {
    log.Fatal(err)
}

// or with a custom file:
if err := goenv.Process(&cfg, ".env.development"); err != nil {
    log.Fatal(err)
}
```

#### Struct Tags

| Tag | Description |
|-----|-------------|
| `envconfig:"KEY"` | Env var name to look up. Required unless `split_words` is set. |
| `split_words:"true"` | Derives the env key from the field name (`CamelCase` → `UPPER_SNAKE_CASE`). Ignored when `envconfig` is also set. |
| `required:"true"` | Returns an error if the env var is unset and no default is provided. |
| `default:"value"` | Fallback value when the env var is unset or empty. |
| `ignored:"true"` | Skips the field entirely. |

Supported field types: `string`, `int`/`int8`/`int16`/`int32`/`int64`, `uint`/`uint8`/`uint16`/`uint32`/`uint64`, `float32`/`float64`, `bool`.

---

## Run Tests

```bash
make test
```

## Build from Source

```bash
make build
```
