package main

import (
	"context"
	"crypto/rand"
	"io"
	"log/slog"
	"net/http"
	"time"
)

const (
	logContextKey contextKey = "log_context"
	reqIDHeader   string     = "X-Request-ID"
)

type LogContext struct {
	Username string
	Error    error
}

func statusToMessage(status int, err error) string {
	switch status {
	case 401, 403, 500:
		return http.StatusText(status)
	default:
		return err.Error()
	}
}

func httpError(ctx context.Context, w http.ResponseWriter, status int, err error) {
	if logCtx, ok := ctx.Value(logContextKey).(*LogContext); ok {
		logCtx.Error = err
	}
	http.Error(w, statusToMessage(status, err), status)
}

type spyReadCloser struct {
	io.ReadCloser
	bytesRead int
}

func (r *spyReadCloser) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	r.bytesRead += n
	return n, err
}

type spyResponseWriter struct {
	http.ResponseWriter
	bytesWritten int
	statusCode   int
}

func (w *spyResponseWriter) Write(p []byte) (int, error) {
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytesWritten += n
	return n, err
}

func (w *spyResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			spyReader := &spyReadCloser{ReadCloser: r.Body}
			r.Body = spyReader
			spyWriter := &spyResponseWriter{ResponseWriter: w}

			logContext := &LogContext{}
			ctx := context.WithValue(r.Context(), logContextKey, logContext)
			r = r.WithContext(ctx)

			next.ServeHTTP(spyWriter, r)
			reqID := r.Context().Value("request_id").(string)
			attrs := []any{
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("client_ip", r.RemoteAddr),
				slog.Duration("duration", time.Since(start)),
				slog.String("request_id", reqID),
				slog.Int("request_body_bytes", spyReader.bytesRead),
				slog.Int("response_status", spyWriter.statusCode),
				slog.Int("response_body_bytes", spyWriter.bytesWritten),
			}
			// log context specifc attrs
			username := logContext.Username
			if username != "" {
				attrs = append(attrs, slog.String("user", username))
			}
			httpError := logContext.Error
			if httpError != nil {
				attrs = append(attrs, slog.Any("error", httpError))
			}
			logger.Info("Served request", attrs...)
		})
	}
}

func requestID() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqID := r.Header.Get(reqIDHeader)
			if reqID == "" {
				reqID = rand.Text()
			}
			w.Header().Set(reqIDHeader, reqID)
			ctx := context.WithValue(r.Context(), "request_id", reqID)
			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
		})
	}
}
