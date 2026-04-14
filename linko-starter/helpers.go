package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"boot.dev/linko/internal/build"
	"boot.dev/linko/internal/linkoerr"
	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
	pkgerr "github.com/pkg/errors"
	"gopkg.in/natefinch/lumberjack.v2"
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
	var noColor bool
	if isatty.IsCygwinTerminal(os.Stderr.Fd()) || isatty.IsTerminal(os.Stderr.Fd()) {
		noColor = true
	}
	debugHandler := tint.NewHandler(os.Stderr, &tint.Options{
		Level:       slog.LevelDebug,
		ReplaceAttr: replaceAttr,
		NoColor:     noColor,
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
	lumberjackLogger := &lumberjack.Logger{
		Filename:   logFile.Name(),
		MaxSize:    1,
		MaxAge:     28,
		MaxBackups: 10,
		LocalTime:  false,
		Compress:   true,
	}
	infoHandler := slog.NewJSONHandler(lumberjackLogger, &slog.HandlerOptions{
		Level:       slog.LevelInfo,
		ReplaceAttr: replaceAttr,
	})
	multiHandler := slog.NewMultiHandler(debugHandler, infoHandler)
	logger := slog.New(multiHandler)
	hostname, err := os.Hostname()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get hostname: %v", err)
	}
	logger = logger.With(slog.String("git_sha", build.GitSHA), slog.String("build_time", build.BuildTime), slog.String("env", os.Getenv("ENV")), slog.String("hostname", hostname))
	closeFn := func() error {
		err := lumberjackLogger.Close()
		if err != nil {
			return fmt.Errorf("failed to close logger %v", err)
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
		var me multiError
		if errors.As(err, &me) {
			errAttrs := errorAttr(me)
			return slog.GroupAttrs("errors", errAttrs...)
		}
		attrs := errorAttrs(err)
		return slog.GroupAttrs("error", attrs...)
	}
	return a
}

func errorAttrs(err error) []slog.Attr {
	var attrs []slog.Attr
	attrs = append(attrs, slog.String("message", err.Error()))
	if st, ok := err.(stackTracer); ok {
		attrs = append(attrs, slog.String("stack_trace", fmt.Sprintf("%+v", st.StackTrace())))
	}
	attrs = append(attrs, linkoerr.Attrs(err)...)
	return attrs
}

func errorAttr(me multiError) []slog.Attr {
	errs := me.Unwrap()
	var errAttrs []slog.Attr
	for i, err := range errs {
		attrs := errorAttrs(err)
		errAttrs = append(errAttrs, slog.GroupAttrs(fmt.Sprintf("error_%d", i+1), attrs...))
	}
	return errAttrs
}
