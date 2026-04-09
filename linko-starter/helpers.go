package main

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"os"
)

type closeFunc func() error

func initLogger(logPath string) (*slog.Logger, closeFunc, error) {
	if logPath == "" {
		closeFn := func() error {
			return nil
		}
		return slog.New(slog.NewTextHandler(os.Stderr, nil)), closeFn, nil
	}
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open log file: %v", err)
	}
	bufferedFile := bufio.NewWriterSize(logFile, 8192)
	mw := io.MultiWriter(bufferedFile, os.Stderr)
	logger := slog.New(slog.NewTextHandler(mw, nil))

	closeFn := func() error {
		err := bufferedFile.Flush()
		if err != nil {
			return fmt.Errorf("failed to flush buffer err: %v", err)
		}
		err = logFile.Close()
		if err != nil {
			return fmt.Errorf("failed to close log file err: %v", err)
		}
		return nil
	}
	return logger, closeFn, nil
}

func closeLogger(closeFn closeFunc) {
	if err := closeFn(); err != nil {
		fmt.Printf("could not close logger err: %v", err)
	}
}
