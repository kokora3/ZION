package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kokora3/zion/internal/board"
	"github.com/kokora3/zion/internal/chain"
	"github.com/kokora3/zion/internal/objects"
	"github.com/kokora3/zion/internal/protocol"
	"github.com/kokora3/zion/internal/research"
	"github.com/kokora3/zion/internal/resources"
)

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "--version" || os.Args[1] == "version") {
		fmt.Fprintln(os.Stdout, protocol.VersionString("zionctl"))
		return
	}

	flags := flag.NewFlagSet("zionctl", flag.ExitOnError)
	apiURL := flags.String("api", "http://127.0.0.1:42001", "local ZION API base URL")
	bearerTokenFile := flags.String("bearer-token-file", "", "read API bearer token from a local file")
	_ = flags.Parse(os.Args[1:])
	args := flags.Args()
	if len(args) == 0 {
		fail("usage: zionctl [--api URL] status|peers|state|tx ...|object ...|board ...|research ...|resource ...")
	}
	client := &http.Client{Timeout: 15 * time.Second}
	var method, path string
	var body io.Reader
	var objectOutput string
	var objectForce bool
	switch args[0] {
	case "status", "peers", "state":
		if len(args) != 1 {
			fail("unexpected arguments")
		}
		method, path = http.MethodGet, "/v1/"+args[0]
	case "tx":
		if len(args) != 3 {
			fail("usage: zionctl tx submit <file> | tx status <txid>")
		}
		switch args[1] {
		case "submit":
			raw, err := readBounded(args[2], chain.MaxTransactionBytes)
			if err != nil {
				fail(err.Error())
			}
			wrapper, _ := json.Marshal(map[string]string{"encoding": "base64", "transaction": base64.StdEncoding.EncodeToString(raw)})
			method, path, body = http.MethodPost, "/v1/transactions", bytes.NewReader(wrapper)
		case "status":
			if _, err := chain.ParseTxID(args[2]); err != nil {
				fail(err.Error())
			}
			method, path = http.MethodGet, "/v1/transactions/"+url.PathEscape(args[2])
		default:
			fail("unknown tx command")
		}
	case "object":
		if len(args) < 3 {
			fail("usage: zionctl object put <file> | get <id> --out <file> [--force] | stat <id> | fetch <id>")
		}
		switch args[1] {
		case "put":
			if len(args) != 3 {
				fail("usage: zionctl object put <file>")
			}
			raw, err := readBoundedFile(args[2], objects.MaxObjectBytes, "object")
			if err != nil {
				fail(err.Error())
			}
			if _, err := objects.Decode(raw); err != nil {
				fail("object file is not a valid canonical ZION object: " + err.Error())
			}
			wrapper, _ := json.Marshal(map[string]string{"encoding": "base64", "object": base64.StdEncoding.EncodeToString(raw)})
			method, path, body = http.MethodPost, "/v1/objects", bytes.NewReader(wrapper)
		case "get":
			if len(args) < 5 {
				fail("usage: zionctl object get <id> --out <file> [--force]")
			}
			id := requireObjectID(args[2])
			for index := 3; index < len(args); index++ {
				switch args[index] {
				case "--out":
					index++
					if index >= len(args) || args[index] == "" || objectOutput != "" {
						fail("--out requires exactly one file path")
					}
					objectOutput = args[index]
				case "--force":
					objectForce = true
				default:
					fail("unknown object get option: " + args[index])
				}
			}
			if objectOutput == "" {
				fail("object get requires --out <file>")
			}
			method, path = http.MethodGet, "/v1/objects/"+url.PathEscape(id)
		case "stat":
			if len(args) != 3 {
				fail("usage: zionctl object stat <id>")
			}
			id := requireObjectID(args[2])
			method, path = http.MethodGet, "/v1/objects/"+url.PathEscape(id)+"/meta"
		case "fetch":
			if len(args) != 3 {
				fail("usage: zionctl object fetch <id>")
			}
			id := requireObjectID(args[2])
			method, path = http.MethodPost, "/v1/objects/"+url.PathEscape(id)+"/fetch"
		default:
			fail("unknown object command")
		}
	case "board":
		if len(args) < 2 {
			fail("usage: zionctl board submit|feed|get|replies|search|hide|unhide")
		}
		switch args[1] {
		case "submit":
			if len(args) != 3 {
				fail("usage: zionctl board submit <signed-event-object-file>")
			}
			raw, err := readBoundedFile(args[2], objects.MaxObjectBytes, "Board event")
			if err != nil {
				fail(err.Error())
			}
			object, err := objects.Decode(raw)
			if err != nil {
				fail("invalid Board event object: " + err.Error())
			}
			if _, err := board.DecodeEventObject(object); err != nil {
				fail("invalid Board event object: " + err.Error())
			}
			wrapper, _ := json.Marshal(map[string]string{"encoding": "base64", "event": base64.StdEncoding.EncodeToString(raw)})
			method, path, body = http.MethodPost, "/v1/board/events", bytes.NewReader(wrapper)
		case "feed":
			if len(args) != 2 {
				fail("usage: zionctl board feed")
			}
			method, path = http.MethodGet, "/v1/board/posts"
		case "get", "replies", "hide", "unhide":
			if len(args) != 3 {
				fail("usage: zionctl board " + args[1] + " <post-id>")
			}
			id := requireObjectID(args[2])
			method, path = http.MethodGet, "/v1/board/posts/"+url.PathEscape(id)
			if args[1] == "replies" {
				path += "/replies"
			} else if args[1] == "hide" || args[1] == "unhide" {
				method, path = http.MethodPost, path+"/"+args[1]
			}
		case "search":
			if len(args) != 3 || args[2] == "" {
				fail("usage: zionctl board search <query>")
			}
			method, path = http.MethodGet, "/v1/board/search?q="+url.QueryEscape(args[2])
		default:
			fail("unknown board command")
		}
	case "research", "resource":
		kind := args[0]
		if len(args) < 2 {
			fail("usage: zionctl " + kind + " list|get|search")
		}
		base := "/v1/" + kind
		if kind == "resource" {
			base = "/v1/resources"
		}
		switch args[1] {
		case "list":
			if len(args) != 2 {
				fail("usage: zionctl " + kind + " list")
			}
			method, path = http.MethodGet, base
		case "get":
			if len(args) != 3 {
				fail("usage: zionctl " + kind + " get <id>")
			}
			if kind == "research" {
				if _, err := research.ParseID(args[2]); err != nil {
					fail("invalid ResearchID: " + err.Error())
				}
			} else {
				if _, err := resources.ParseID(args[2]); err != nil {
					fail("invalid ResourceID: " + err.Error())
				}
			}
			method, path = http.MethodGet, base+"/"+url.PathEscape(args[2])
		case "search":
			if len(args) != 3 || args[2] == "" {
				fail("usage: zionctl " + kind + " search <query>")
			}
			method, path = http.MethodGet, base+"/search?q="+url.QueryEscape(args[2])
		default:
			fail("unknown " + kind + " command")
		}
	default:
		fail("unknown command")
	}
	request, err := http.NewRequest(method, strings.TrimRight(*apiURL, "/")+path, body)
	if err != nil {
		fail(err.Error())
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if *bearerTokenFile != "" {
		tokenBytes, tokenErr := readBounded(*bearerTokenFile, 4096)
		if tokenErr != nil {
			fail(tokenErr.Error())
		}
		token := strings.TrimSpace(string(tokenBytes))
		if len(token) < 16 {
			fail("bearer token must contain at least 16 non-whitespace bytes")
		}
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.Do(request)
	if err != nil {
		fail(err.Error())
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 2*objects.MaxObjectBytes+1))
	if err != nil {
		fail(err.Error())
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		fmt.Fprintln(os.Stdout, string(data))
		os.Exit(1)
	}
	if objectOutput != "" {
		if err := writeObjectResponse(data, objectOutput, objectForce); err != nil {
			fail(err.Error())
		}
		fmt.Fprintf(os.Stdout, "wrote canonical object bytes to %s\n", objectOutput)
		return
	}
	fmt.Fprintln(os.Stdout, string(data))
}

func readBounded(path string, maximum int) ([]byte, error) {
	return readBoundedFile(path, maximum, "transaction")
}

func readBoundedFile(path string, maximum int, kind string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s input must be a regular file", kind)
	}
	if info.Size() < 1 || info.Size() > int64(maximum) {
		return nil, fmt.Errorf("%s file size exceeds limit", kind)
	}
	data, err := io.ReadAll(io.LimitReader(file, int64(maximum)+1))
	if err != nil {
		return nil, err
	}
	if len(data) < 1 || len(data) > maximum {
		return nil, fmt.Errorf("%s file size exceeds limit", kind)
	}
	return data, nil
}

func requireObjectID(value string) string {
	id, err := protocol.ParseObjectID(value)
	if err != nil {
		fail("invalid ObjectID: " + err.Error())
	}
	return id.String()
}

func writeObjectResponse(data []byte, destination string, force bool) error {
	var wrapper struct {
		Encoding string `json:"encoding"`
		Object   string `json:"object"`
		ObjectID string `json:"object_id"`
		Size     int    `json:"size"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wrapper); err != nil || wrapper.Encoding != "base64" {
		return fmt.Errorf("invalid object API response")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("invalid trailing object API response data")
	}
	raw, err := base64.StdEncoding.DecodeString(wrapper.Object)
	if err != nil || len(raw) > objects.MaxObjectBytes {
		return fmt.Errorf("invalid object bytes in API response")
	}
	object, err := objects.Decode(raw)
	if err != nil {
		return fmt.Errorf("object API response failed validation: %w", err)
	}
	id, err := object.ObjectID()
	if err != nil || id.String() != wrapper.ObjectID {
		return fmt.Errorf("object API response ObjectID mismatch")
	}
	if wrapper.Size != len(raw) {
		return fmt.Errorf("object API response size mismatch")
	}
	clean := filepath.Clean(destination)
	flags := os.O_WRONLY | os.O_CREATE
	if force {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_EXCL
	}
	file, err := os.OpenFile(clean, flags, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("output exists; pass --force to replace it")
		}
		return err
	}
	if _, err := file.Write(raw); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, "zionctl:", message)
	os.Exit(2)
}
