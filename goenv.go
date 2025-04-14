package goenv

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

func readFile(filepath string, ch chan string) []string {
	fileBytes, err := os.ReadFile(filepath)
	if err != nil {
		ch <- fmt.Sprintf("Failed to open file: %v\n", err)
	}
	return strings.Split(string(fileBytes), "\n")
}

func addEnvs(endPos int, chunkSize int, entries []string, ch chan string, name string, wg *sync.WaitGroup) {
	defer wg.Done()
	slice := entries[endPos-chunkSize : endPos]
	for i := range len(slice) {
		before, after, _ := strings.Cut(slice[i], "=")
		err := os.Setenv(before, after)
		if err != nil {
			ch <- fmt.Sprintf("%s Error\n", name)
		}
	}
	ch <- fmt.Sprintf("%s Complete\n", name)
}

func LoadEnv(filepath string, verbose bool) {
	start := time.Now()
	numOfCpus := runtime.NumCPU()
	channel := make(chan string, 50)
	var wg sync.WaitGroup
	envFile := readFile(filepath, channel)
	if len(envFile) < 20 {
		wg.Add(1)
		go addEnvs(len(envFile), len(envFile), envFile, channel, "Thread 1", &wg)
	} else {
		envChunkSize := len(envFile) / (numOfCpus / 2)
		numOfThreads := len(envFile) / envChunkSize
		for i := 1; i < numOfThreads+1; i++ {
			wg.Add(1)
			go addEnvs(envChunkSize*1, envChunkSize, envFile, channel, fmt.Sprintf("Thead %d", i), &wg)
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
