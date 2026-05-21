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
		Handler: NewReverseProxyHandler(store, opts.ServiceName),
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

func NewReverseProxyHandler(store *Store, serviceName string) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		target, err := activeProxyTarget(store, serviceName)
		if err != nil {
			http.Error(response, err.Error(), http.StatusBadGateway)
			return
		}

		proxy := httputil.NewSingleHostReverseProxy(target)
		director := proxy.Director
		proxy.Director = func(proxyRequest *http.Request) {
			originalHost := proxyRequest.Host
			director(proxyRequest)
			proxyRequest.Host = originalHost
		}
		proxy.ErrorHandler = func(response http.ResponseWriter, request *http.Request, err error) {
			http.Error(response, fmt.Sprintf("proxy to %s failed: %v", target.String(), err), http.StatusBadGateway)
		}
		proxy.ServeHTTP(response, request)
	})
}

func activeProxyTarget(store *Store, serviceName string) (*url.URL, error) {
	state, err := store.Load()
	if err != nil {
		return nil, err
	}
	service, instance, err := ActiveInstance(state, serviceName)
	if err != nil {
		return nil, err
	}
	if service.ListenPort <= 0 {
		return nil, fmt.Errorf("service %q does not have a listen port", serviceName)
	}
	if instance.URL == "" {
		return nil, fmt.Errorf("active instance %q for service %q has no URL", instance.ID, serviceName)
	}
	target, err := url.Parse(instance.URL)
	if err != nil {
		return nil, fmt.Errorf("parse active instance URL %q: %w", instance.URL, err)
	}
	return target, nil
}
