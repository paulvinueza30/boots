package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"boot.dev/linko/internal/linkoerr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
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
			clientIP := r.RemoteAddr
			if redactedIP, err := redactIP(r.RemoteAddr); err == nil && redactedIP != nil {
				clientIP = *redactedIP
			} else if err != nil {
				logContext.Error = linkoerr.WithAttrs(err, slog.String("http error", err.Error()))
			}
			attrs := []any{
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("client_ip", clientIP),
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

func redactIP(ip string) (*string, error) {
	host, _, err := net.SplitHostPort(ip)
	if err != nil {
		return nil, fmt.Errorf("could not split ip %v", err)
	}
	parsed := net.ParseIP(host)
	if parsed == nil {
		return nil, fmt.Errorf("invalid IP: %s", host)
	}
	redacted := parsed.Mask(net.IPv4Mask(255, 255, 255, 0)).String()
	if parsed.To4() != nil {
		lastOctet := net.ParseIP(host).To4()[3]
		redacted += "." + strings.Repeat("x", int(lastOctet))
	}
	return &redacted, nil
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// httpRequestsTotal counts requests by method, path and status.
var httpRequestsTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total number of HTTP requests.",
	},
	[]string{"method", "path", "status"},
)

func metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{
			ResponseWriter: w,
			status:         http.StatusOK,
		}

		next.ServeHTTP(rec, r)

		path := r.URL.Path
		method := r.Method
		status := strconv.Itoa(rec.status)

		httpRequestsTotal.
			WithLabelValues(method, path, status).
			Inc()
	})
}
