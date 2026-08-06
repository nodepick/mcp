// Package main is the entry point for the mcpd service.
// It supports both foreground (stdio/sse) mode and background daemonization.
package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	custom_mcp "mcpd/internal/mcp"
	"mcpd/internal/tools"

	official_mcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func generateSelfSignedCert() (tls.Certificate, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return tls.Certificate{}, err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"nodepick-mcpd"},
			CommonName:   "localhost",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	privBytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privBytes})

	return tls.X509KeyPair(certPEM, keyPEM)
}

func authMiddleware(apiKey string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if apiKey == "" {
			next.ServeHTTP(w, r)
			return
		}

		// Support OPTIONS preflight requests for CORS
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		token := r.Header.Get("X-API-Key")
		if token == "" {
			authHeader := r.Header.Get("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				token = strings.TrimPrefix(authHeader, "Bearer ")
			}
		}
		if token == "" {
			token = r.URL.Query().Get("api_key")
		}
		if token == "" {
			token = r.URL.Query().Get("token")
		}

		if token != apiKey {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"status":"error","error":"Unauthorized: Invalid or missing API key"}`))
			return
		}

		next.ServeHTTP(w, r)
	})
}

func main() {
	// 1. Define CLI Flags
	foreground := flag.Bool("foreground", false, "Run the server in the foreground")
	flag.BoolVar(foreground, "f", false, "Run the server in the foreground (shorthand)")

	transport := flag.String("transport", "", "Transport protocol to use: 'stdio' or 'http'. (Defaults to 'stdio' in foreground, 'http' in daemon)")
	flag.StringVar(transport, "t", "", "Transport protocol (shorthand)")

	port := flag.String("port", "8000", "Port to bind the HTTP server to")
	flag.StringVar(port, "p", "8000", "Port (shorthand)")

	host := flag.String("host", "", "Host address to bind the HTTP server to. (Defaults to '127.0.0.1' in foreground, '0.0.0.0' in daemon)")
	flag.StringVar(host, "h", "", "Host address (shorthand)")

	logPath := flag.String("log", "/var/log/mcpd.log", "Path to log file when running in background daemon mode")

	apiKey := flag.String("api-key", "", "API key required to authenticate HTTP/SSE requests (optional)")
	apiKeyFile := flag.String("api-key-file", "/usr/local/etc/mcpd.conf", "Path to file containing the API key (optional)")
	tlsCert := flag.String("tls-cert", "", "Path to custom TLS certificate PEM file (optional)")
	tlsKey := flag.String("tls-key", "", "Path to custom TLS private key PEM file (optional)")

	flag.Parse()

	// Resolve API key
	resolvedAPIKey := *apiKey
	if resolvedAPIKey == "" {
		if _, err := os.Stat(*apiKeyFile); err == nil {
			data, err := os.ReadFile(*apiKeyFile)
			if err == nil {
				resolvedAPIKey = strings.TrimSpace(string(data))
				slog.Info("Loaded API key from configuration file", "path", *apiKeyFile)
			} else {
				slog.Error("Failed to read API key file", "path", *apiKeyFile, "error", err)
				os.Exit(1)
			}
		} else if *apiKeyFile != "/usr/local/etc/mcpd.conf" {
			slog.Error("API key file not found", "path", *apiKeyFile)
			os.Exit(1)
		}
	}

	// 2. Resolve Default Values based on Execution Mode
	isDaemonChild := os.Getenv("_MCPD_DAEMON") == "1"
	isForeground := *foreground || isDaemonChild

	// Resolve Transport
	resolvedTransport := *transport
	if resolvedTransport == "" {
		if isForeground && !isDaemonChild {
			resolvedTransport = "stdio"
		} else {
			resolvedTransport = "http"
		}
	}

	// Resolve Host Binding
	resolvedHost := *host
	if resolvedHost == "" {
		if isForeground && !isDaemonChild {
			resolvedHost = "127.0.0.1"
		} else {
			resolvedHost = "0.0.0.0"
		}
	}

	// 3. Handle Background Daemonization
	if !isForeground {
		exe, err := os.Executable()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to find executable path: %v\n", err)
			os.Exit(1)
		}

		// Prepare command arguments, ensuring flags are passed to the daemonized child
		cmd := exec.Command(exe, os.Args[1:]...)
		cmd.Env = append(os.Environ(), "_MCPD_DAEMON=1")

		// Attempt to open the log file, falling back to /tmp if permissions are denied
		logFile, err := os.OpenFile(*logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0640)
		if err != nil {
			// Fallback to tmp
			fallbackPath := "/tmp/mcpd.log"
			logFile, err = os.OpenFile(fallbackPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				logFile, _ = os.OpenFile(os.DevNull, os.O_WRONLY, 0)
				*logPath = "/dev/null"
			} else {
				*logPath = fallbackPath
			}
		}
		defer logFile.Close()

		// Detach stdio and redirect standard output/error of daemon to log file
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		cmd.Stdin = nil

		if err := cmd.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to start background daemon process: %v\n", err)
			os.Exit(1)
		}

		// Print clean summary to stdout and exit parent process
		fmt.Printf("mcpd starting in background (pid: %d, log: %s)\n", cmd.Process.Pid, *logPath)
		os.Exit(0)
	}

	// 4. Configure Structured Logger (Stderr is redirected to log file in daemon mode)
	level := slog.LevelInfo
	if l := os.Getenv("LOG_LEVEL"); l != "" {
		switch l {
		case "DEBUG":
			level = slog.LevelDebug
		case "WARN":
			level = slog.LevelWarn
		case "ERROR":
			level = slog.LevelError
		}
	}

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	if isDaemonChild {
		slog.Info("mcpd daemon process starting", "pid", os.Getpid(), "transport", resolvedTransport)
	} else {
		slog.Info("mcpd foreground process starting", "pid", os.Getpid(), "transport", resolvedTransport)
	}

	// 5. Setup Context and Signal Handling for Graceful Shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		slog.Info("Received OS shutdown signal", "signal", sig.String())
		cancel()
	}()

	// 6. Instantiate the official MCP Server
	officialServer := official_mcp.NewServer(&official_mcp.Implementation{
		Name:    "mcpd",
		Version: "1.0.0",
	}, &official_mcp.ServerOptions{
		Capabilities: &official_mcp.ServerCapabilities{},
	})

	// Define adapter functions to map custom types to official mcp types
	adaptTool := func(t custom_mcp.Tool) *official_mcp.Tool {
		return &official_mcp.Tool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		}
	}

	adaptHandler := func(h custom_mcp.ToolHandler) official_mcp.ToolHandler {
		return func(ctx context.Context, req *official_mcp.CallToolRequest) (*official_mcp.CallToolResult, error) {
			customResult, err := h(ctx, req.Params.Arguments)
			if err != nil {
				return nil, err
			}

			officialResult := &official_mcp.CallToolResult{
				IsError: customResult.IsError,
			}
			for _, content := range customResult.Content {
				officialResult.Content = append(officialResult.Content, &official_mcp.TextContent{
					Text: content.Text,
				})
			}
			return officialResult, nil
		}
	}

	// 7. Register System Management Tools
	officialServer.AddTool(adaptTool(tools.GetPackageManageToolDefinition()), adaptHandler(tools.PackageManageHandler))
	officialServer.AddTool(adaptTool(tools.GetUserManageToolDefinition()), adaptHandler(tools.UserManageHandler))
	officialServer.AddTool(adaptTool(tools.GetServicesToolDefinition()), adaptHandler(tools.ServicesHandler))
	officialServer.AddTool(adaptTool(tools.GetCommandExecToolDefinition()), adaptHandler(tools.CommandExecHandler))

	for _, toolDef := range tools.GetFilesToolDefinitions() {
		var handler custom_mcp.ToolHandler
		switch toolDef.Name {
		case "file_list":
			handler = tools.FileListHandler
		case "file_read":
			handler = tools.FileReadHandler
		case "file_write":
			handler = tools.FileWriteHandler
		case "file_patch":
			handler = tools.FilePatchHandler
		}
		officialServer.AddTool(adaptTool(toolDef), adaptHandler(handler))
	}

	// 8. Launch the Chosen Transport Loop
	if resolvedTransport == "stdio" {
		slog.Info("Starting MCP stdio transport loop")
		if err := officialServer.Run(ctx, &official_mcp.StdioTransport{}); err != nil {
			slog.Error("Stdio server stopped with error", "error", err)
			os.Exit(1)
		}
	} else if resolvedTransport == "http" || resolvedTransport == "sse" {
		var handler http.Handler
		if resolvedTransport == "sse" {
			slog.Info("Initializing SSE handler")
			handler = official_mcp.NewSSEHandler(func(r *http.Request) *official_mcp.Server {
				return officialServer
			}, &official_mcp.SSEOptions{})
		} else {
			slog.Info("Initializing Streamable HTTP handler")
			handler = official_mcp.NewStreamableHTTPHandler(func(r *http.Request) *official_mcp.Server {
				return officialServer
			}, &official_mcp.StreamableHTTPOptions{
				Logger: logger,
			})
		}

		if resolvedAPIKey != "" {
			slog.Info("Authentication enabled via API Key")
			handler = authMiddleware(resolvedAPIKey, handler)
		}

		srv := &http.Server{
			Addr:    resolvedHost + ":" + *port,
			Handler: handler,
		}

		var cert tls.Certificate
		var err error
		if *tlsCert != "" && *tlsKey != "" {
			slog.Info("Loading custom TLS certificate pair", "cert", *tlsCert, "key", *tlsKey)
			cert, err = tls.LoadX509KeyPair(*tlsCert, *tlsKey)
		} else {
			slog.Info("Generating transient self-signed ECDSA certificate for HTTPS...")
			cert, err = generateSelfSignedCert()
		}
		if err != nil {
			slog.Error("Failed to configure TLS", "error", err)
			os.Exit(1)
		}

		srv.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},

			// Minimum TLS version 1.2
			MinVersion: tls.VersionTLS12,
			// Allow post-quantum key exchange as preferred, with fallbacks to standard curves for compatibility with LibreSSL/3.3.6
			CurvePreferences: []tls.CurveID{
				tls.X25519MLKEM768,
				tls.X25519,
				tls.CurveP256,
				tls.CurveP384,
				tls.CurveP521,
			},
		}

		// Setup graceful HTTP server shutdown on cancellation context
		go func() {
			<-ctx.Done()
			slog.Info("Shutting down HTTP server...")
			_ = srv.Shutdown(context.Background())
		}()

		slog.Info("Starting HTTPS server", "transport", resolvedTransport, "host", resolvedHost, "port", *port)
		if err := srv.ListenAndServeTLS("", ""); err != http.ErrServerClosed {
			slog.Error("HTTPS server failed", "error", err)
			os.Exit(1)
		}
	} else {
		slog.Error("Unsupported transport protocol", "transport", resolvedTransport)
		os.Exit(1)
	}

	slog.Info("mcpd server shutdown complete")
}
