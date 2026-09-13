package api

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kokora3/zion/internal/board"
	"github.com/kokora3/zion/internal/index"
	"github.com/kokora3/zion/internal/objects"
	"github.com/kokora3/zion/internal/protocol"
	"github.com/kokora3/zion/internal/research"
	"github.com/kokora3/zion/internal/resources"
)

const (
	BasePath           = "/v1"
	DefaultListen      = "127.0.0.1:42001"
	DefaultMaxBody     = 96 * 1024
	HardMaxBody        = 128 * 1024
	DefaultConcurrency = 32
	HardMaxConcurrency = 128
	MaxPageSize        = 100
	// MaxObjectBody allows one base64-encoded MaxObjectBytes object plus a
	// small, fixed JSON envelope. Transaction body limits remain unchanged.
	MaxObjectBody = ((objects.MaxObjectBytes + 2) / 3 * 4) + 1024
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
	PutObject(context.Context, objects.Object) (protocol.ObjectID, objects.PutResult, error)
	GetObject(context.Context, protocol.ObjectID) (objects.Object, error)
	StatObject(context.Context, protocol.ObjectID) (objects.Metadata, error)
	FetchObject(context.Context, protocol.ObjectID) (objects.Object, error)
	SubmitBoardEvent(context.Context, []byte) (any, error)
	BoardFeed(offset, limit int) (any, error)
	BoardPost(string) (any, bool, error)
	BoardReplies(postID string, offset, limit int) (any, error)
	BoardSearch(query string, offset, limit int) (any, error)
	SetBoardHidden(string, bool) error
	ResearchList(offset, limit int) (any, error)
	Research(string) (any, bool, error)
	ResearchSearch(query string, offset, limit int) (any, error)
	ResourceList(offset, limit int) (any, error)
	Resource(string) (any, bool, error)
	ResourceSearch(query string, offset, limit int) (any, error)
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
		case BasePath + "/board/posts":
			offset, limit, err := pagination(request)
			if err != nil {
				s.writeError(writer, http.StatusBadRequest, "INVALID_PAGINATION", err.Error())
				return
			}
			value, err := s.backend.BoardFeed(offset, limit)
			if err != nil {
				s.writeBoardError(writer, err)
				return
			}
			s.writeJSON(writer, http.StatusOK, value)
			return
		case BasePath + "/board/search":
			offset, limit, err := pagination(request)
			if err != nil {
				s.writeError(writer, http.StatusBadRequest, "INVALID_PAGINATION", err.Error())
				return
			}
			value, err := s.backend.BoardSearch(request.URL.Query().Get("q"), offset, limit)
			if err != nil {
				s.writeBoardError(writer, err)
				return
			}
			s.writeJSON(writer, http.StatusOK, value)
			return
		case BasePath + "/research", BasePath + "/resources":
			offset, limit, err := pagination(request)
			if err != nil {
				s.writeError(writer, http.StatusBadRequest, "INVALID_PAGINATION", err.Error())
				return
			}
			var value any
			if path == BasePath+"/research" {
				value, err = s.backend.ResearchList(offset, limit)
			} else {
				value, err = s.backend.ResourceList(offset, limit)
			}
			if err != nil {
				s.writeError(writer, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
				return
			}
			s.writeJSON(writer, http.StatusOK, value)
			return
		case BasePath + "/research/search", BasePath + "/resources/search":
			offset, limit, err := pagination(request)
			if err != nil {
				s.writeError(writer, http.StatusBadRequest, "INVALID_PAGINATION", err.Error())
				return
			}
			var value any
			if path == BasePath+"/research/search" {
				value, err = s.backend.ResearchSearch(request.URL.Query().Get("q"), offset, limit)
			} else {
				value, err = s.backend.ResourceSearch(request.URL.Query().Get("q"), offset, limit)
			}
			if err != nil {
				s.writeError(writer, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
				return
			}
			s.writeJSON(writer, http.StatusOK, value)
			return
		}
		for _, route := range []struct {
			prefix string
			parse  func(string) error
			get    func(string) (any, bool, error)
		}{
			{BasePath + "/research/", func(value string) error { _, err := research.ParseID(value); return err }, s.backend.Research},
			{BasePath + "/resources/", func(value string) error { _, err := resources.ParseID(value); return err }, s.backend.Resource},
		} {
			if strings.HasPrefix(path, route.prefix) {
				id := strings.TrimPrefix(path, route.prefix)
				if id == "" || strings.Contains(id, "/") || route.parse(id) != nil {
					s.writeError(writer, http.StatusBadRequest, "INVALID_ID", "invalid registry ID")
					return
				}
				value, found, err := route.get(id)
				if err != nil {
					s.writeError(writer, http.StatusBadRequest, "INVALID_ID", err.Error())
					return
				}
				if !found {
					s.writeError(writer, http.StatusNotFound, "NOT_FOUND", "registry entry not found")
					return
				}
				s.writeJSON(writer, http.StatusOK, value)
				return
			}
		}
		if strings.HasPrefix(path, BasePath+"/board/posts/") {
			s.getBoardPost(writer, request)
			return
		}
		if strings.HasPrefix(path, BasePath+"/objects/") {
			s.getObject(writer, request)
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
	if path == BasePath+"/objects" && request.Method == http.MethodPost {
		s.putObject(writer, request)
		return
	}
	if request.Method == http.MethodPost && strings.HasPrefix(path, BasePath+"/objects/") && strings.HasSuffix(path, "/fetch") {
		s.fetchObject(writer, request)
		return
	}
	if path == BasePath+"/board/events" && request.Method == http.MethodPost {
		s.submitBoardEvent(writer, request)
		return
	}
	if request.Method == http.MethodPost && strings.HasPrefix(path, BasePath+"/board/posts/") {
		s.setBoardVisibility(writer, request)
		return
	}
	if strings.HasPrefix(path, BasePath+"/") {
		s.writeError(writer, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method is not supported")
		return
	}
	s.writeError(writer, http.StatusNotFound, "NOT_FOUND", "endpoint not found")
}

func (s *Server) submitBoardEvent(writer http.ResponseWriter, request *http.Request) {
	data, ok := s.readObjectBody(writer, request)
	if !ok {
		return
	}
	var wrapper struct {
		Encoding string `json:"encoding"`
		Event    string `json:"event"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wrapper); err != nil || wrapper.Encoding != "base64" || wrapper.Event == "" {
		s.writeError(writer, http.StatusBadRequest, "INVALID_BOARD_EVENT", "expected one base64 signed Board event object")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF || len(wrapper.Event) > base64.StdEncoding.EncodedLen(objects.MaxObjectBytes) {
		s.writeError(writer, http.StatusBadRequest, "INVALID_BOARD_EVENT", "invalid or oversized Board event wrapper")
		return
	}
	if len(wrapper.Event) > base64.StdEncoding.EncodedLen(board.MaxBoardEventObjectBytes) {
		s.writeError(writer, http.StatusRequestEntityTooLarge, "BOARD_EVENT_TOO_LARGE", "Board event exceeds the hard limit")
		return
	}
	raw, err := base64.StdEncoding.DecodeString(wrapper.Event)
	if err != nil || len(raw) > board.MaxBoardEventObjectBytes {
		s.writeError(writer, http.StatusBadRequest, "INVALID_BOARD_EVENT", "invalid base64 Board event")
		return
	}
	result, err := s.backend.SubmitBoardEvent(request.Context(), raw)
	if err != nil {
		s.writeBoardError(writer, err)
		return
	}
	s.writeJSON(writer, http.StatusCreated, result)
}

func (s *Server) getBoardPost(writer http.ResponseWriter, request *http.Request) {
	prefix := BasePath + "/board/posts/"
	value := strings.TrimPrefix(request.URL.Path, prefix)
	if strings.HasSuffix(value, "/replies") {
		id := strings.TrimSuffix(value, "/replies")
		if _, err := protocol.ParseObjectID(id); err != nil {
			s.writeError(writer, http.StatusBadRequest, "INVALID_POST_ID", "invalid Board post ID")
			return
		}
		offset, limit, err := pagination(request)
		if err != nil {
			s.writeError(writer, http.StatusBadRequest, "INVALID_PAGINATION", err.Error())
			return
		}
		result, err := s.backend.BoardReplies(id, offset, limit)
		if err != nil {
			s.writeBoardError(writer, err)
			return
		}
		s.writeJSON(writer, http.StatusOK, result)
		return
	}
	if value == "" || strings.Contains(value, "/") {
		s.writeError(writer, http.StatusBadRequest, "INVALID_POST_ID", "invalid Board post ID")
		return
	}
	if _, err := protocol.ParseObjectID(value); err != nil {
		s.writeError(writer, http.StatusBadRequest, "INVALID_POST_ID", "invalid Board post ID")
		return
	}
	result, found, err := s.backend.BoardPost(value)
	if err != nil {
		s.writeBoardError(writer, err)
		return
	}
	if !found {
		s.writeError(writer, http.StatusNotFound, "BOARD_POST_NOT_FOUND", "Board post not found")
		return
	}
	s.writeJSON(writer, http.StatusOK, result)
}

func (s *Server) setBoardVisibility(writer http.ResponseWriter, request *http.Request) {
	prefix := BasePath + "/board/posts/"
	value := strings.TrimPrefix(request.URL.Path, prefix)
	hidden := false
	switch {
	case strings.HasSuffix(value, "/hide"):
		hidden = true
		value = strings.TrimSuffix(value, "/hide")
	case strings.HasSuffix(value, "/unhide"):
		value = strings.TrimSuffix(value, "/unhide")
	default:
		s.writeError(writer, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method is not supported")
		return
	}
	if _, err := protocol.ParseObjectID(value); err != nil {
		s.writeError(writer, http.StatusBadRequest, "INVALID_POST_ID", "invalid Board post ID")
		return
	}
	if err := s.backend.SetBoardHidden(value, hidden); err != nil {
		s.writeBoardError(writer, err)
		return
	}
	status := "VISIBLE"
	if hidden {
		status = "LOCALLY_HIDDEN"
	}
	s.writeJSON(writer, http.StatusOK, map[string]any{"post_id": value, "local_visibility": status})
}

func (s *Server) writeBoardError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, board.ErrPublicationDenied):
		s.writeError(writer, http.StatusForbidden, "BOARD_PUBLICATION_DENIED", "ACTIVE membership and the current signing key are required")
	case errors.Is(err, board.ErrParentNotFound):
		s.writeError(writer, http.StatusUnprocessableEntity, "BOARD_PARENT_NOT_FOUND", "reply parent is not available locally")
	case errors.Is(err, board.ErrBoardContentMissing):
		s.writeError(writer, http.StatusUnprocessableEntity, "BOARD_CONTENT_MISSING", "primary Board content is not available locally")
	case errors.Is(err, board.ErrReferenceNotFound):
		s.writeError(writer, http.StatusUnprocessableEntity, "BOARD_REFERENCE_NOT_FOUND", "canonical registry reference is not admitted")
	case errors.Is(err, board.ErrInvalidContent):
		s.writeError(writer, http.StatusUnprocessableEntity, "INVALID_BOARD_CONTENT", "Board content is invalid")
	case errors.Is(err, board.ErrInvalidEvent):
		s.writeError(writer, http.StatusUnprocessableEntity, "INVALID_BOARD_EVENT", "Board event is invalid")
	case errors.Is(err, index.ErrInvalidBoardQuery):
		s.writeError(writer, http.StatusBadRequest, "INVALID_BOARD_QUERY", "Board query is invalid or exceeds limits")
	case errors.Is(err, objects.ErrStoreFull):
		s.writeError(writer, http.StatusInsufficientStorage, "OBJECT_STORE_FULL", "local object-store quota exceeded")
	case errors.Is(err, objects.ErrObjectNotFound), errors.Is(err, os.ErrNotExist):
		s.writeError(writer, http.StatusNotFound, "BOARD_POST_NOT_FOUND", "Board post not found")
	default:
		s.writeError(writer, http.StatusInternalServerError, "BOARD_ERROR", "Board operation failed")
	}
}

func (s *Server) putObject(writer http.ResponseWriter, request *http.Request) {
	data, ok := s.readObjectBody(writer, request)
	if !ok {
		return
	}
	var wrapper struct {
		Encoding string `json:"encoding"`
		Object   string `json:"object"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wrapper); err != nil || wrapper.Encoding != "base64" || wrapper.Object == "" {
		s.writeError(writer, http.StatusBadRequest, "INVALID_OBJECT", "expected one base64 object wrapper")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		s.writeError(writer, http.StatusBadRequest, "INVALID_OBJECT", "trailing request data")
		return
	}
	if len(wrapper.Object) > base64.StdEncoding.EncodedLen(objects.MaxObjectBytes) {
		s.writeError(writer, http.StatusRequestEntityTooLarge, "OBJECT_TOO_LARGE", "object exceeds hard size limit")
		return
	}
	raw, err := base64.StdEncoding.DecodeString(wrapper.Object)
	if err != nil {
		s.writeError(writer, http.StatusBadRequest, "INVALID_OBJECT", "invalid base64 object")
		return
	}
	object, err := objects.Decode(raw)
	if err != nil {
		s.writeObjectError(writer, err)
		return
	}
	id, result, err := s.backend.PutObject(request.Context(), object)
	if err != nil {
		s.writeObjectError(writer, err)
		return
	}
	status := http.StatusCreated
	if result == objects.PutAlreadyExists {
		status = http.StatusOK
	}
	s.writeJSON(writer, status, map[string]any{"object_id": id.String(), "size": len(raw), "status": result})
}

func (s *Server) getObject(writer http.ResponseWriter, request *http.Request) {
	id, meta, ok := parseObjectPath(request.URL.Path, "")
	if !ok {
		id, meta, ok = parseObjectPath(request.URL.Path, "/meta")
	}
	if !ok {
		s.writeError(writer, http.StatusBadRequest, "INVALID_OBJECT_ID", "invalid object path")
		return
	}
	if meta {
		value, err := s.backend.StatObject(request.Context(), id)
		if err != nil {
			s.writeObjectError(writer, err)
			return
		}
		s.writeJSON(writer, http.StatusOK, map[string]any{"object_id": value.ObjectID.String(), "size": value.Size,
			"present": true, "stored_at": value.StoredAt.UTC().Format(time.RFC3339Nano)})
		return
	}
	object, err := s.backend.GetObject(request.Context(), id)
	if err != nil {
		s.writeObjectError(writer, err)
		return
	}
	raw, err := object.CanonicalBytes()
	if err != nil {
		s.writeObjectError(writer, err)
		return
	}
	s.writeJSON(writer, http.StatusOK, map[string]any{"encoding": "base64", "object": base64.StdEncoding.EncodeToString(raw),
		"object_id": id.String(), "size": len(raw)})
}

func (s *Server) fetchObject(writer http.ResponseWriter, request *http.Request) {
	id, _, ok := parseObjectPath(request.URL.Path, "/fetch")
	if !ok {
		s.writeError(writer, http.StatusBadRequest, "INVALID_OBJECT_ID", "invalid ObjectID")
		return
	}
	if _, err := s.backend.GetObject(request.Context(), id); err == nil {
		meta, statErr := s.backend.StatObject(request.Context(), id)
		if statErr != nil {
			s.writeObjectError(writer, statErr)
			return
		}
		s.writeJSON(writer, http.StatusOK, map[string]any{"object_id": id.String(), "size": meta.Size, "status": "ALREADY_LOCAL"})
		return
	} else if !errors.Is(err, objects.ErrObjectNotFound) {
		s.writeObjectError(writer, err)
		return
	}
	object, err := s.backend.FetchObject(request.Context(), id)
	if err != nil {
		s.writeObjectError(writer, err)
		return
	}
	raw, err := object.CanonicalBytes()
	if err != nil {
		s.writeObjectError(writer, err)
		return
	}
	actualID, err := object.ObjectID()
	if err != nil || actualID.String() != id.String() {
		s.writeObjectError(writer, objects.ErrInvalidRemoteResponse)
		return
	}
	s.writeJSON(writer, http.StatusOK, map[string]any{"object_id": id.String(), "size": len(raw), "status": "FETCHED"})
}

func parseObjectPath(path, suffix string) (protocol.ObjectID, bool, bool) {
	prefix := BasePath + "/objects/"
	if !strings.HasPrefix(path, prefix) {
		return protocol.ObjectID{}, false, false
	}
	value := strings.TrimPrefix(path, prefix)
	meta := suffix == "/meta"
	if suffix != "" {
		if !strings.HasSuffix(value, suffix) {
			return protocol.ObjectID{}, false, false
		}
		value = strings.TrimSuffix(value, suffix)
	} else if strings.Contains(value, "/") {
		return protocol.ObjectID{}, false, false
	}
	if value == "" || strings.Contains(value, "/") {
		return protocol.ObjectID{}, false, false
	}
	id, err := protocol.ParseObjectID(value)
	if err != nil {
		return protocol.ObjectID{}, false, false
	}
	return id, meta, true
}

func (s *Server) readObjectBody(writer http.ResponseWriter, request *http.Request) ([]byte, bool) {
	data, err := io.ReadAll(io.LimitReader(request.Body, MaxObjectBody+1))
	if err != nil || len(data) > MaxObjectBody {
		s.writeError(writer, http.StatusRequestEntityTooLarge, "BODY_TOO_LARGE", "request body exceeds object API limit")
		return nil, false
	}
	return data, true
}

func (s *Server) writeObjectError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, objects.ErrObjectNotFound):
		s.writeError(writer, http.StatusNotFound, "OBJECT_NOT_FOUND", "object not found")
	case errors.Is(err, objects.ErrObjectCorrupt):
		s.writeError(writer, http.StatusInternalServerError, "OBJECT_CORRUPT", "stored object failed integrity verification")
	case errors.Is(err, objects.ErrStoreFull):
		s.writeError(writer, http.StatusInsufficientStorage, "OBJECT_STORE_FULL", "local object-store quota exceeded")
	case errors.Is(err, objects.ErrObjectTooLarge):
		s.writeError(writer, http.StatusRequestEntityTooLarge, "OBJECT_TOO_LARGE", "object exceeds hard size limit")
	case errors.Is(err, objects.ErrInvalidObject):
		s.writeError(writer, http.StatusUnprocessableEntity, "INVALID_OBJECT", "object is malformed or non-canonical")
	case errors.Is(err, objects.ErrObjectFetchTimeout), errors.Is(err, context.DeadlineExceeded):
		s.writeError(writer, http.StatusGatewayTimeout, "OBJECT_FETCH_TIMEOUT", "object fetch timed out")
	case errors.Is(err, objects.ErrNoAvailablePeers):
		s.writeError(writer, http.StatusServiceUnavailable, "NO_OBJECT_PEERS", "no compatible object peers are available")
	case errors.Is(err, objects.ErrInvalidRemoteResponse):
		s.writeError(writer, http.StatusBadGateway, "INVALID_REMOTE_OBJECT", "remote object response failed validation")
	default:
		s.writeError(writer, http.StatusInternalServerError, "OBJECT_STORE_ERROR", "object operation failed")
	}
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
