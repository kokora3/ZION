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
	"strings"
	"time"

	"github.com/kokora3/zion/internal/chain"
	"github.com/kokora3/zion/internal/protocol"
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
		fail("usage: zionctl [--api URL] status|peers|state|tx submit <file>|tx status <txid>")
	}
	client := &http.Client{Timeout: 15 * time.Second}
	var method, path string
	var body io.Reader
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
	data, err := io.ReadAll(io.LimitReader(response.Body, 1024*1024))
	if err != nil {
		fail(err.Error())
	}
	fmt.Fprintln(os.Stdout, string(data))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		os.Exit(1)
	}
}

func readBounded(path string, maximum int) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() < 1 || info.Size() > int64(maximum) {
		return nil, fmt.Errorf("transaction file size exceeds limit")
	}
	return io.ReadAll(file)
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, "zionctl:", message)
	os.Exit(2)
}
