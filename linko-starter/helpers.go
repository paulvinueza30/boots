package main

import (
	"io"
	"log"
	"os"
)

func initLogger() *log.Logger {
	logPath, exists := os.LookupEnv("LINKO_LOG_FILE")
	if !exists {
		return log.New(os.Stderr, "", log.LstdFlags)
	}
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		log.Fatalf("failed to open log file: %v", err)
	}
	mw := io.MultiWriter(logFile, os.Stderr)
	logger := log.New(mw, "", log.LstdFlags)
	return logger
}
