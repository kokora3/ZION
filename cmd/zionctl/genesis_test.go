package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenesisVerifyCommand(t *testing.T) {
	root := filepath.Join("..", "..", "configs", "alpha-1")
	var output bytes.Buffer
	err := runGenesisCommand([]string{"verify",
		"--manifest", filepath.Join(root, "validators-public.json"),
		"--genesis", filepath.Join(root, "genesis.json"),
		"--genesis-id", filepath.Join(root, "genesis-id.txt"), "--json"}, &output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"validation": "PASS"`) ||
		!strings.Contains(output.String(), `"genesis_id": "72ef0c7816d64255fc7b1da266c6a5b5ca345dd89cba2423fc8cfc8b4d1a61c6"`) {
		t.Fatalf("unexpected verification output: %s", output.String())
	}
}
