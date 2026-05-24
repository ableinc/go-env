# Go env

A simple library to import environment variables from a file into your program at runtime. For large `.env` files, it will split the env file into even chunks based on the number of CPUs on your machine, then add the variables to your environment. If the same key appears multiple times, the last occurrence wins to reduce collisions.

## How to use

1. Add to your project
```bash
go get github.com/ableinc/go-env
```

2. Import in your project
```go
import "github.com/ableinc/go-env"

goenv.LoadEnv(".env", false)
```

## Run Tests

```bash
make test
```

## Build from Source

```bash
make build
```
