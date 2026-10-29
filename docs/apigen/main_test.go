package main

import (
	"bytes"
	"os"
	"testing"
)

// TestAPIDataIsCurrent fails when docs/gift.dev/data/api.json does not match
// the source, so that the API tables of the site cannot fall behind silently.
func TestAPIDataIsCurrent(t *testing.T) {
	want, err := generate(os.DirFS("../.."))
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../gift.dev/data/api.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("docs/gift.dev/data/api.json is out of date, run: go run ./docs/apigen")
	}
}
