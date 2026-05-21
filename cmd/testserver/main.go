package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"
)

func main() {
	var id string
	var port int
	flag.StringVar(&id, "id", envDefault("WP_ID", "testserver"), "server id")
	flag.IntVar(&port, "port", envIntDefault("PORT", 0), "port to listen on; defaults to PORT or a random port")
	flag.Parse()

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()
	log.Printf("wp testserver %s listening on http://%s", id, addr)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := map[string]string{
			"id":     id,
			"addr":   addr,
			"host":   request.Host,
			"method": request.Method,
			"path":   request.URL.Path,
			"time":   time.Now().Format(time.RFC3339),
		}
		_ = json.NewEncoder(w).Encode(response)
	})

	server := &http.Server{Handler: mux}
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Fatalf("serve: %v", err)
	}
}

func envDefault(name string, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value
}

func envIntDefault(name string, fallback int) int {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
