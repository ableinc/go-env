package goenv

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

// readFile reads the content of the .env file and returns it as a slice of lines.
func readFile(filepath string, ch chan string) []string {
	fileBytes, err := os.ReadFile(filepath)
	if err != nil {
		ch <- fmt.Sprintf("Failed to open file: %v", err)
		return nil
	}
	return strings.Split(string(fileBytes), "\n")
}

// filterLines filters out empty lines and comments (starting with '#').
func filterLines(lines []string) []string {
	var filtered []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		filtered = append(filtered, line)
	}
	return filtered
}

// setEnvFromLines sets environment variables from a slice of lines.
func setEnvFromLines(lines []string, ch chan string, name string, wg *sync.WaitGroup) {
	defer wg.Done()
	for _, line := range lines {
		before, after, found := strings.Cut(line, "=")
		if found && before != "" {
			err := os.Setenv(strings.TrimSpace(before), strings.Trim(strings.TrimSpace(after), `"`))
			if err != nil {
				ch <- fmt.Sprintf("%s: Error setting %s", name, before)
			}
		}
	}
	ch <- fmt.Sprintf("%s Complete", name)
}

// LoadEnv loads environment variables from a .env file using parallel goroutines.
func LoadEnv(filepath string, verbose bool) {
	start := time.Now()
	numOfCpus := runtime.NumCPU()
	channel := make(chan string, 50)
	var wg sync.WaitGroup

	rawLines := readFile(filepath, channel)
	envFile := filterLines(rawLines)

	if len(envFile) == 0 {
		channel <- "No valid environment variables found."
		close(channel)
		return
	}

	// If short file, just run in a single goroutine
	if len(envFile) < 20 {
		wg.Add(1)
		go setEnvFromLines(envFile, channel, "Thread 1", &wg)
	} else {
		chunkSize := len(envFile) / (numOfCpus / 2)
		if chunkSize == 0 {
			chunkSize = 1
		}
		for i := 0; i < len(envFile); i += chunkSize {
			end := i + chunkSize
			if end > len(envFile) {
				end = len(envFile)
			}
			wg.Add(1)
			go setEnvFromLines(envFile[i:end], channel, fmt.Sprintf("Thread %d", i/chunkSize+1), &wg)
		}
	}

	wg.Wait()
	close(channel)

	end := time.Now()
	if verbose {
		for message := range channel {
			fmt.Println(message)
		}
		fmt.Printf("Completion time: %f seconds\n", end.Sub(start).Seconds())
	}
}
