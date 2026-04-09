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

	// PIN/Security routes (always available, even when locked)
	mux.HandleFunc("/api/pin/status", s.api.HandlePINStatus)
	mux.HandleFunc("/api/pin/setup", s.api.HandlePINSetup)
	mux.HandleFunc("/api/pin/verify", s.api.HandlePINVerify)
	mux.HandleFunc("/api/panic", s.api.HandlePanic)

	// API routes
	mux.HandleFunc("/api/identity", s.api.HandleIdentity)
	mux.HandleFunc("/api/identity/name", s.api.HandleSetName)
	mux.HandleFunc("/api/contacts", s.api.HandleContacts)
	mux.HandleFunc("/api/messages/", s.api.HandleMessages)
	mux.HandleFunc("/api/status", s.api.HandleStatus)
	mux.HandleFunc("/api/import", s.api.HandleImport)
	mux.HandleFunc("/api/restore", s.api.HandleRestore)
	mux.HandleFunc("/api/online", s.api.HandleOnline)
	mux.HandleFunc("/api/chat/delete/", s.api.HandleDeleteChat)
	mux.HandleFunc("/api/contacts/rename/", s.api.HandleRenameContact)
	mux.HandleFunc("/api/groups", s.api.HandleGroups)
	mux.HandleFunc("/api/groups/messages/", s.api.HandleGroupMessages)
	mux.HandleFunc("/api/groups/delete/", s.api.HandleDeleteGroup)
	mux.HandleFunc("/api/unread", s.api.HandleUnread)
	mux.HandleFunc("/api/update/check", s.api.HandleCheckUpdate)
	mux.HandleFunc("/api/update/download", s.api.HandleUpdateDownload)
	mux.HandleFunc("/api/file/send/", s.api.HandleSendFile)
	mux.HandleFunc("/api/mesh/add-peer", s.api.HandleAddPeer)
	mux.HandleFunc("/api/master/login", s.api.HandleMasterLogin)
	mux.HandleFunc("/api/master/contact", s.api.HandleMasterCheck)
	mux.HandleFunc("/api/lara/contact", s.api.HandleLaraCheck)
	mux.HandleFunc("/api/letters/", s.api.HandleLetters)
	mux.HandleFunc("/api/telemetry/enabled", s.api.HandleTelemetryEnabled)
	mux.HandleFunc("/api/relays", s.api.HandleRelays)
	mux.HandleFunc("/api/channels", s.api.HandleListChannels)
	mux.HandleFunc("/api/channels/create", s.api.HandleCreateChannel)
	mux.HandleFunc("/api/channels/posts/", s.api.HandleChannelPosts)
	mux.HandleFunc("/api/channels/post/", s.api.HandlePostToChannel)
	mux.HandleFunc("/api/channels/subscribe", s.api.HandleSubscribeChannel)
	mux.HandleFunc("/api/channels/unsubscribe", s.api.HandleUnsubscribeChannel)

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
