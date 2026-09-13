package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/kokora3/zion/internal/board"
	"github.com/kokora3/zion/internal/index"
)

func TestBoardAPIContract(t *testing.T) {
	storeObject := apiTestObject("phase-10-api-post-id")
	postID, _ := storeObject.ObjectID()
	entry := index.BoardEntry{PostID: postID.String(), Kind: board.KindPost, LocalVisibility: "VISIBLE"}
	backend := &objectBackend{boardResult: map[string]any{"post_id": postID.String(), "status": "STORED"},
		boardValue: []index.BoardEntry{entry}, boardFound: true}
	server := newObjectServer(t, backend)
	raw := []byte{0xa1, 0x01, 0x01}
	wrapper, _ := json.Marshal(map[string]string{"encoding": "base64", "event": base64.StdEncoding.EncodeToString(raw)})
	response := objectRequest(t, server, http.MethodPost, BasePath+"/board/events", wrapper)
	if response.Code != http.StatusCreated || !bytes.Equal(backend.boardRaw, raw) || !strings.Contains(response.Body.String(), postID.String()) {
		t.Fatalf("submit = %d %s", response.Code, response.Body.String())
	}
	for _, path := range []string{
		BasePath + "/board/posts",
		BasePath + "/board/posts/" + postID.String(),
		BasePath + "/board/posts/" + postID.String() + "/replies",
		BasePath + "/board/search?q=phase-10",
	} {
		response = objectRequest(t, server, http.MethodGet, path, nil)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), postID.String()) {
			t.Fatalf("GET %s = %d %s", path, response.Code, response.Body.String())
		}
	}
	response = objectRequest(t, server, http.MethodPost, BasePath+"/board/posts/"+postID.String()+"/hide", nil)
	if response.Code != http.StatusOK || backend.hiddenID != postID.String() || !backend.hidden {
		t.Fatalf("hide = %d %s", response.Code, response.Body.String())
	}
	response = objectRequest(t, server, http.MethodPost, BasePath+"/board/posts/"+postID.String()+"/unhide", nil)
	if response.Code != http.StatusOK || backend.hidden {
		t.Fatalf("unhide = %d %s", response.Code, response.Body.String())
	}
}

func TestBoardAPIRejectsMalformedBoundedAndSanitizesErrors(t *testing.T) {
	backend := &objectBackend{}
	server := newObjectServer(t, backend)
	cases := []struct {
		method string
		path   string
		body   []byte
		code   int
	}{
		{http.MethodPost, BasePath + "/board/events", []byte(`{"encoding":"base64","event":"AQ==","unknown":true}`), http.StatusBadRequest},
		{http.MethodGet, BasePath + "/board/posts/not-an-id", nil, http.StatusBadRequest},
		{http.MethodGet, BasePath + "/board/posts?limit=101", nil, http.StatusBadRequest},
	}
	for _, test := range cases {
		response := objectRequest(t, server, test.method, test.path, test.body)
		if response.Code != test.code {
			t.Fatalf("%s %s = %d %s", test.method, test.path, response.Code, response.Body.String())
		}
	}
	backend.boardErr = index.ErrInvalidBoardQuery
	response := objectRequest(t, server, http.MethodGet, BasePath+"/board/search", nil)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "INVALID_BOARD_QUERY") {
		t.Fatalf("empty search = %d %s", response.Code, response.Body.String())
	}
	backend.boardErr = errors.New("secret filesystem C:\\private\\board-index")
	response = objectRequest(t, server, http.MethodGet, BasePath+"/board/posts", nil)
	if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), "filesystem") || strings.Contains(response.Body.String(), "private") {
		t.Fatalf("unsanitized error = %d %s", response.Code, response.Body.String())
	}
	backend.boardErr = board.ErrPublicationDenied
	wrapper, _ := json.Marshal(map[string]string{"encoding": "base64", "event": base64.StdEncoding.EncodeToString([]byte{1})})
	response = objectRequest(t, server, http.MethodPost, BasePath+"/board/events", wrapper)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "BOARD_PUBLICATION_DENIED") {
		t.Fatalf("publication denial = %d %s", response.Code, response.Body.String())
	}
}
