package main

import (
	"bufio"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"boot.dev/linko/internal/linkoerr"
	pkgerr "github.com/pkg/errors"
)

type closeFunc func() error

type stackTracer interface {
	error
	StackTrace() pkgerr.StackTrace
}

type multiError interface {
	error
	Unwrap() []error
}

func initLogger(logPath string) (*slog.Logger, closeFunc, error) {
	debugHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level:       slog.LevelDebug,
		ReplaceAttr: replaceAttr,
	})
	if logPath == "" {
		closeFn := func() error {
			return nil
		}
		return slog.New(debugHandler), closeFn, nil
	}
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open log file: %v", err)
	}
	bufferedFile := bufio.NewWriterSize(logFile, 8192)
	infoHandler := slog.NewJSONHandler(bufferedFile, &slog.HandlerOptions{
		Level:       slog.LevelInfo,
		ReplaceAttr: replaceAttr,
	})
	multiHandler := slog.NewMultiHandler(debugHandler, infoHandler)
	logger := slog.New(multiHandler)

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

func replaceAttr(groups []string, a slog.Attr) slog.Attr {
	if a.Key == "error" {
		err, ok := a.Value.Any().(error)
		if !ok {
			return a
		}
		if multiError, ok := errors.AsType[multiError](err); ok {
			errAttrs := errorAttr(multiError)
			return slog.GroupAttrs("errors", errAttrs...)
		}
		attrs := linkoerr.Attrs(err)
		if len(attrs) > 0 {
			return slog.GroupAttrs("error", attrs...)
		}
	}
	return a
}

func errorAttr(me multiError) []slog.Attr {
	errs := me.Unwrap()
	var errAttrs []slog.Attr
	for i, err := range errs {
		attrs := linkoerr.Attrs(err)
		errAttrs = append(errAttrs, slog.GroupAttrs(fmt.Sprintf("error_%d", i+1), attrs...))
	}
	return errAttrs
}
