package goenv

import (
	"bytes"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"
)

type envPair struct {
	key   string
	value string
}

const MAX_ENV_THRESHOLD = 50

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
	deduped := make([]envPair, 0, len(pairs))
	for i := len(pairs) - 1; i >= 0; i-- {
		if _, exists := seen[pairs[i].key]; exists {
			continue
		}
		seen[pairs[i].key] = struct{}{}
		deduped = append(deduped, pairs[i])
	}
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
