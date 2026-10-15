package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp_project/pkg/research"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	statelessFlag := flag.Bool("stateless", false, "run server in stateless HTTP mode (no session persistence)")
	flag.Parse()

	// Initialize the research MCP server
	server := research.NewServer()

	// 1. Streamable HTTP Handler
	// Stateful (default): maintains session state across requests using Mcp-Session-Id
	// Stateless (-stateless=true): temporary session per request, no memory between calls
	streamableOpts := &mcp.StreamableHTTPOptions{
		Stateless:                  *statelessFlag,
		DisableLocalhostProtection: true, // Allow remote connections across domains/proxies
	}
	streamableHandler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return server
	}, streamableOpts)

	// 2. SSE Handler (for backwards-compatibility with SSE clients)
	sseHandler := mcp.NewSSEHandler(func(r *http.Request) *mcp.Server {
		return server
	}, nil)

	mux := http.NewServeMux()

	// Health check endpoint (for Render.com / cloud container orchestration)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Streamable HTTP endpoints
	mux.Handle("/mcp", streamableHandler)
	mux.Handle("/mcp/", streamableHandler)

	// SSE endpoints
	mux.Handle("/sse", sseHandler)
	mux.Handle("/sse/", sseHandler)

	addr := ":" + port
	httpServer := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	mode := "Stateful"
	if *statelessFlag {
		mode = "Stateless"
	}

	log.Printf("=================================================================")
	log.Printf("Starting Remote Research MCP Server (%s mode)", mode)
	log.Printf("Listening on:            http://0.0.0.0:%s", port)
	log.Printf("Streamable HTTP Endpoint: http://0.0.0.0:%s/mcp", port)
	log.Printf("SSE Endpoint:            http://0.0.0.0:%s/sse", port)
	log.Printf("Health Check:            http://0.0.0.0:%s/health", port)
	log.Printf("=================================================================")

	// Graceful shutdown handling
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	<-stop
	log.Println("\nShutting down remote MCP server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("Error during server shutdown: %v", err)
	} else {
		log.Println("Server gracefully stopped.")
	}
}
