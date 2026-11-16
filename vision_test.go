package main_test

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/aerth/mostly/mllm"
	"github.com/aerth/orchard-tui/pkg/visions"
)

func getTestImage(t *testing.T) (imageb []byte) {
	if _, err := os.Stat("tmp/placeholder.png"); err == nil {
		b, err := os.ReadFile("tmp/placeholder.png")
		if err != nil {
			panic(err.Error())
		}
		return b
	}
	placeholderURL := "https://picsum.photos/id/26/4209/2769"
	r, err := http.Get(placeholderURL)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("tmp", 0o755); err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile("tmp/placeholder.png", b, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestVisionAttach(t *testing.T) {
	imageb := getTestImage(t)
	msg := mllm.Message{
		Role:    "user",
		Content: "what are the objects [./tmp/placeholder.png]",
	}
	err := visions.AttachImages(msg.Content, &msg)
	if err != nil {
		t.Fatal(err)
	}
	if len(msg.Images) != 1 {
		t.Fatalf("no images attached: %d", len(msg.Images))
	}
	if !bytes.Equal(imageb, msg.Images[0]) {
		t.Fatalf("image changed: %q != %q", msg.Images[0], imageb)
	}
}

func TestVisionReply(t *testing.T) {
	imageb := getTestImage(t)
	msg := mllm.Message{
		Role:    "user",
		Content: "what are the objects [./tmp/placeholder.png]",
	}
	err := visions.AttachImages(msg.Content, &msg)
	if err != nil {
		t.Fatal(err)
	}
	if len(msg.Images) != 1 {
		t.Fatalf("no images attached: %d", len(msg.Images))
	}
	if !bytes.Equal(imageb, msg.Images[0]) {
		t.Fatalf("image changed: %q != %q", msg.Images[0], imageb)
	}
	client := mllm.Default()
	if err := client.ValidateConfig(nil); err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	got, err := client.Get(ctx, mllm.Messages{msg}, false)
	if err != nil {
		t.Fatal(err)
	}
	_ = got
}
