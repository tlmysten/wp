package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

type ServeOptions struct {
	ServiceName string
	Host        string
	Stdout      io.Writer
}

func ServeService(ctx context.Context, store *Store, opts ServeOptions) error {
	if opts.ServiceName == "" {
		return fmt.Errorf("service name is required")
	}
	state, err := store.Load()
	if err != nil {
		return err
	}
	service, ok := state.Services[opts.ServiceName]
	if !ok {
		return fmt.Errorf("unknown service %q", opts.ServiceName)
	}
	if service.ListenPort <= 0 {
		return fmt.Errorf("service %q does not have a listen port", opts.ServiceName)
	}

	host := opts.Host
	if host == "" {
		host = "127.0.0.1"
	}
	stdout := opts.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}

	addr := fmt.Sprintf("%s:%d", host, service.ListenPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}
	defer listener.Close()

	server := &http.Server{
		Handler: NewReverseProxyHandler(store, opts.ServiceName, stdout),
	}
	go func() {
		<-ctx.Done()
		_ = server.Close()
	}()

	fmt.Fprintf(stdout, "serving %s on %s\n", service.Name, addr)
	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func NewReverseProxyHandler(store *Store, serviceName string, logOutput io.Writer) http.Handler {
	logger := &proxyLogger{out: logOutput}
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		startedAt := time.Now()
		recorder := &loggingResponseWriter{ResponseWriter: response}
		instanceID := "-"
		targetURL := "-"
		proxyErr := ""
		defer func() {
			logger.Log(proxyRequestLog{
				Time:      startedAt,
				Service:   serviceName,
				Instance:  instanceID,
				Method:    request.Method,
				Path:      request.URL.RequestURI(),
				Status:    recorder.Status(),
				Bytes:     recorder.Bytes(),
				Duration:  time.Since(startedAt),
				TargetURL: targetURL,
				Error:     proxyErr,
			})
		}()

		target, err := activeProxyTarget(store, serviceName)
		if err != nil {
			proxyErr = err.Error()
			http.Error(recorder, err.Error(), http.StatusBadGateway)
			return
		}
		instanceID = target.Instance.ID
		targetURL = target.URL.String()

		proxy := httputil.NewSingleHostReverseProxy(target.URL)
		director := proxy.Director
		proxy.Director = func(proxyRequest *http.Request) {
			originalHost := proxyRequest.Host
			director(proxyRequest)
			proxyRequest.Host = originalHost
		}
		proxy.ErrorHandler = func(response http.ResponseWriter, request *http.Request, err error) {
			proxyErr = err.Error()
			http.Error(response, fmt.Sprintf("proxy to %s failed: %v", target.URL.String(), err), http.StatusBadGateway)
		}
		proxy.ServeHTTP(recorder, request)
	})
}

type proxyTarget struct {
	Instance Instance
	URL      *url.URL
}

func activeProxyTarget(store *Store, serviceName string) (proxyTarget, error) {
	state, err := store.Load()
	if err != nil {
		return proxyTarget{}, err
	}
	service, instance, err := ActiveInstance(state, serviceName)
	if err != nil {
		return proxyTarget{}, err
	}
	if service.ListenPort <= 0 {
		return proxyTarget{}, fmt.Errorf("service %q does not have a listen port", serviceName)
	}
	if instance.URL == "" {
		return proxyTarget{}, fmt.Errorf("active instance %q for service %q has no URL", instance.ID, serviceName)
	}
	target, err := url.Parse(instance.URL)
	if err != nil {
		return proxyTarget{}, fmt.Errorf("parse active instance URL %q: %w", instance.URL, err)
	}
	return proxyTarget{Instance: instance, URL: target}, nil
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (writer *loggingResponseWriter) WriteHeader(status int) {
	if writer.status != 0 {
		return
	}
	writer.status = status
	writer.ResponseWriter.WriteHeader(status)
}

func (writer *loggingResponseWriter) Write(data []byte) (int, error) {
	if writer.status == 0 {
		writer.WriteHeader(http.StatusOK)
	}
	count, err := writer.ResponseWriter.Write(data)
	writer.bytes += int64(count)
	return count, err
}

func (writer *loggingResponseWriter) Unwrap() http.ResponseWriter {
	return writer.ResponseWriter
}

func (writer *loggingResponseWriter) Status() int {
	if writer.status == 0 {
		return http.StatusOK
	}
	return writer.status
}

func (writer *loggingResponseWriter) Bytes() int64 {
	return writer.bytes
}

type proxyLogger struct {
	out io.Writer
	mu  sync.Mutex
}

type proxyRequestLog struct {
	Time      time.Time
	Service   string
	Instance  string
	Method    string
	Path      string
	Status    int
	Bytes     int64
	Duration  time.Duration
	TargetURL string
	Error     string
}

func (logger *proxyLogger) Log(entry proxyRequestLog) {
	if logger.out == nil {
		return
	}
	fields := []string{
		entry.Time.Format(time.RFC3339),
		fmt.Sprintf("service=%s", entry.Service),
		fmt.Sprintf("id=%s", entry.Instance),
		fmt.Sprintf("method=%s", entry.Method),
		fmt.Sprintf("path=%s", entry.Path),
		fmt.Sprintf("status=%d", entry.Status),
		fmt.Sprintf("bytes=%d", entry.Bytes),
		fmt.Sprintf("duration=%s", entry.Duration.Round(time.Millisecond)),
		fmt.Sprintf("target=%s", entry.TargetURL),
	}
	if entry.Error != "" {
		fields = append(fields, fmt.Sprintf("error=%q", entry.Error))
	}

	logger.mu.Lock()
	defer logger.mu.Unlock()
	fmt.Fprintln(logger.out, strings.Join(fields, " "))
}
