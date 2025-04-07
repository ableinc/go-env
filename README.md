# Go env

A simple library to import environment variables from a file into your program at runtime. For large (files with more than 20 variables) it will split the env file into even chunks based on number of CPUs on your machine, then add the variables to your environment. 

## How to use

1. Add to your project
```bash
go get github.com/ableinc/go-env
```

2. Import in your project
```go
import "github.com/ableinc/go-env"

LoadEnv(".env", false)
```