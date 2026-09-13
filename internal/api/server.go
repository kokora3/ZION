package api

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	BasePath           = "/v1"
	DefaultListen      = "127.0.0.1:42001"
	DefaultMaxBody     = 96 * 1024
	HardMaxBody        = 128 * 1024
	DefaultConcurrency = 32
	HardMaxConcurrency = 128
	MaxPageSize        = 100
)

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Submission struct {
	TxID   string `json:"tx_id"`
	Status string `json:"status"`
}

type Backend interface {
	Health() any
	Status() any
	Peers() any
	StateSummary() any
	Identity(string) (any, bool, error)
	Membership(string) (any, bool, error)
	Proposals(offset, limit int) (any, error)
	Proposal(string) (any, bool, error)
	SubmitTransaction(context.Context, []byte) (Submission, error)
	Transaction(string) (any, bool, error)
}

type Config struct {
	Listen         string
	AllowedOrigins []string
	BearerToken    string
	MaxBodyBytes   int64
	MaxConcurrent  int
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	IdleTimeout    time.Duration
}

func DefaultConfig() Config {
	return Config{Listen: DefaultListen, MaxBodyBytes: DefaultMaxBody, MaxConcurrent: DefaultConcurrency,
		ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
}

type Server struct {
	cfg      Config
	backend  Backend
	server   *http.Server
	listener net.Listener
	sem      chan struct{}
}

func NewServer(cfg Config, backend Backend) (*Server, error) {
	if backend == nil || cfg.Listen == "" || cfg.MaxBodyBytes < 1 || cfg.MaxBodyBytes > HardMaxBody ||
		cfg.MaxConcurrent < 1 || cfg.MaxConcurrent > HardMaxConcurrency || cfg.ReadTimeout <= 0 || cfg.WriteTimeout <= 0 || cfg.IdleTimeout <= 0 {
		return nil, fmt.Errorf("invalid API configuration")
	}
	for _, origin := range cfg.AllowedOrigins {
		if origin == "" || origin == "*" {
			return nil, fmt.Errorf("wildcard or empty CORS origin is forbidden")
		}
	}
	host, _, err := net.SplitHostPort(cfg.Listen)
	if err != nil {
		return nil, fmt.Errorf("invalid API listen address: %w", err)
	}
	ip := net.ParseIP(host)
	if (ip == nil || !ip.IsLoopback()) && len(cfg.BearerToken) < 16 {
		return nil, fmt.Errorf("non-loopback API binding requires a bearer token of at least 16 bytes")
	}
	return &Server{cfg: cfg, backend: backend, sem: make(chan struct{}, cfg.MaxConcurrent)}, nil
}

func (s *Server) Start() error {
	listener, err := net.Listen("tcp", s.cfg.Listen)
	if err != nil {
		return err
	}
	s.listener = listener
	s.server = &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: s.cfg.ReadTimeout,
		WriteTimeout: s.cfg.WriteTimeout, IdleTimeout: s.cfg.IdleTimeout, MaxHeaderBytes: 16 * 1024}
	go func() { _ = s.server.Serve(listener) }()
	return nil
}

func (s *Server) Addr() string {
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

func (s *Server) Close(ctx context.Context) error {
	if s.server == nil {
		return nil
	}
	err := s.server.Shutdown(ctx)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "application/json")
	if len(request.URL.RequestURI()) > 2048 {
		s.writeError(writer, http.StatusRequestURITooLong, "REQUEST_TOO_LONG", "request URI exceeds limit")
		return
	}
	if origin := request.Header.Get("Origin"); origin != "" {
		if !contains(s.cfg.AllowedOrigins, origin) {
			s.writeError(writer, http.StatusForbidden, "ORIGIN_FORBIDDEN", "origin is not allowed")
			return
		}
		writer.Header().Set("Access-Control-Allow-Origin", origin)
		writer.Header().Set("Vary", "Origin")
		if request.Method == http.MethodOptions && strings.HasPrefix(request.URL.Path, BasePath+"/") {
			writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			writer.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			writer.WriteHeader(http.StatusNoContent)
			return
		}
	}
	if !s.authorized(request) {
		s.writeError(writer, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	default:
		s.writeError(writer, http.StatusServiceUnavailable, "BUSY", "API concurrency limit reached")
		return
	}
	s.route(writer, request)
}

func (s *Server) route(writer http.ResponseWriter, request *http.Request) {
	path := request.URL.Path
	if request.Method == http.MethodGet {
		switch path {
		case BasePath + "/health":
			s.writeJSON(writer, http.StatusOK, s.backend.Health())
			return
		case BasePath + "/status":
			s.writeJSON(writer, http.StatusOK, s.backend.Status())
			return
		case BasePath + "/peers":
			s.writeJSON(writer, http.StatusOK, s.backend.Peers())
			return
		case BasePath + "/state":
			s.writeJSON(writer, http.StatusOK, s.backend.StateSummary())
			return
		case BasePath + "/governance/proposals":
			offset, limit, err := pagination(request)
			if err != nil {
				s.writeError(writer, http.StatusBadRequest, "INVALID_PAGINATION", err.Error())
				return
			}
			value, err := s.backend.Proposals(offset, limit)
			if err != nil {
				s.writeError(writer, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
				return
			}
			s.writeJSON(writer, http.StatusOK, value)
			return
		}
		for _, route := range []struct {
			prefix string
			get    func(string) (any, bool, error)
		}{
			{BasePath + "/identities/", s.backend.Identity}, {BasePath + "/memberships/", s.backend.Membership},
			{BasePath + "/governance/proposals/", s.backend.Proposal}, {BasePath + "/transactions/", s.backend.Transaction},
		} {
			if strings.HasPrefix(path, route.prefix) {
				id := strings.TrimPrefix(path, route.prefix)
				if id == "" || strings.Contains(id, "/") {
					s.writeError(writer, http.StatusBadRequest, "INVALID_ID", "invalid path identifier")
					return
				}
				value, found, err := route.get(id)
				if err != nil {
					s.writeError(writer, http.StatusBadRequest, "INVALID_ID", err.Error())
					return
				}
				if !found {
					s.writeError(writer, http.StatusNotFound, "NOT_FOUND", "resource not found")
					return
				}
				s.writeJSON(writer, http.StatusOK, value)
				return
			}
		}
	}
	if path == BasePath+"/transactions" && request.Method == http.MethodPost {
		s.submit(writer, request)
		return
	}
	if strings.HasPrefix(path, BasePath+"/") {
		s.writeError(writer, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method is not supported")
		return
	}
	s.writeError(writer, http.StatusNotFound, "NOT_FOUND", "endpoint not found")
}

func (s *Server) submit(writer http.ResponseWriter, request *http.Request) {
	body := io.LimitReader(request.Body, s.cfg.MaxBodyBytes+1)
	data, err := io.ReadAll(body)
	if err != nil || int64(len(data)) > s.cfg.MaxBodyBytes {
		s.writeError(writer, http.StatusRequestEntityTooLarge, "BODY_TOO_LARGE", "request body exceeds limit")
		return
	}
	var wrapper struct {
		Encoding    string `json:"encoding"`
		Transaction string `json:"transaction"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wrapper); err != nil || wrapper.Encoding != "base64" {
		s.writeError(writer, http.StatusBadRequest, "INVALID_TRANSACTION", "expected one base64 transaction wrapper")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		s.writeError(writer, http.StatusBadRequest, "INVALID_TRANSACTION", "trailing request data")
		return
	}
	raw, err := base64.StdEncoding.DecodeString(wrapper.Transaction)
	if err != nil {
		s.writeError(writer, http.StatusBadRequest, "INVALID_TRANSACTION", "invalid base64 transaction")
		return
	}
	result, err := s.backend.SubmitTransaction(request.Context(), raw)
	if err != nil {
		s.writeError(writer, http.StatusUnprocessableEntity, "INVALID_TRANSACTION", err.Error())
		return
	}
	s.writeJSON(writer, http.StatusAccepted, result)
}

func pagination(request *http.Request) (int, int, error) {
	offset, limit := 0, 50
	var err error
	if value := request.URL.Query().Get("offset"); value != "" {
		offset, err = strconv.Atoi(value)
		if err != nil || offset < 0 {
			return 0, 0, fmt.Errorf("invalid offset")
		}
	}
	if value := request.URL.Query().Get("limit"); value != "" {
		limit, err = strconv.Atoi(value)
		if err != nil || limit < 1 || limit > MaxPageSize {
			return 0, 0, fmt.Errorf("invalid limit")
		}
	}
	return offset, limit, nil
}

func (s *Server) authorized(request *http.Request) bool {
	if s.cfg.BearerToken == "" {
		return true
	}
	want := "Bearer " + s.cfg.BearerToken
	got := request.Header.Get("Authorization")
	return len(want) == len(got) && subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1
}

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
func (s *Server) writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
func (s *Server) writeError(writer http.ResponseWriter, status int, code, message string) {
	s.writeJSON(writer, status, Error{Code: code, Message: message})
}
