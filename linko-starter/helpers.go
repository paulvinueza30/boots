package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
)

func initLogger(logPath string) (*log.Logger, error) {
	if logPath == "" {
		return log.New(os.Stderr, "", log.LstdFlags), nil
	}
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %v", err)
	}
	bufferedFile := bufio.NewWriterSize(logFile, 8192)
	mw := io.MultiWriter(bufferedFile, os.Stderr)
	logger := log.New(mw, "", log.LstdFlags)
	return logger, nil
}
