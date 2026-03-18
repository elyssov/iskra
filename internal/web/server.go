package web

import (
	"embed"
	"fmt"
	"io/fs"
	"net"
	"net/http"
)

//go:embed static/*
var staticFiles embed.FS

// Server is the HTTP server for the Iskra UI.
type Server struct {
	api      *API
	listener net.Listener
	port     int
}

// NewServer creates a new web server.
func NewServer(api *API, port int) *Server {
	return &Server{api: api, port: port}
}

// Start begins serving HTTP requests.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/api/identity", s.api.HandleIdentity)
	mux.HandleFunc("/api/contacts", s.api.HandleContacts)
	mux.HandleFunc("/api/messages/", s.api.HandleMessages)
	mux.HandleFunc("/api/status", s.api.HandleStatus)
	mux.HandleFunc("/api/import", s.api.HandleImport)

	// Static files
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return fmt.Errorf("failed to access static files: %w", err)
	}
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", s.port))
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	s.listener = ln
	s.port = ln.Addr().(*net.TCPAddr).Port

	go http.Serve(ln, mux)
	return nil
}

// Port returns the actual port the server is listening on.
func (s *Server) Port() int {
	return s.port
}

// Stop stops the server.
func (s *Server) Stop() {
	if s.listener != nil {
		s.listener.Close()
	}
}
