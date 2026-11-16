package main

import (
	"log/slog"
	"os"
	"time"

	"github.com/aerth/mostly/mllm"
	"github.com/aerth/mostly/stackerr"
	"github.com/aerth/orchard/pkg/rege"
)

// AttachImages scans text and adds images based on bracketed image path pattern
//
// Example: "No, the button is still weird looking. [/path/to/screenshot.png] see?"
func AttachImages(v string, m *mllm.Message) error {
	paths := rege.FindImagePaths(v)
	if len(paths) == 0 {
		slog.Debug("no images found")
		return nil
	}
	return AttachImagesPaths(paths, m)
}

func AttachImagesPaths(paths []string, m *mllm.Message) error {
	for _, v := range paths {
		b, err := os.ReadFile(v)
		if err != nil {
			go global.SendFunc(stackerr.Errorf("%w", err))
			//return stackerr.Errorf("read image file for attachment: %v", err) // TODO: might be foo.png false positive
		}
		m.Images = append(m.Images, b)
		slog.Info("found image")
		time.Sleep(time.Second)

	}
	return nil
}

func (m *model) addPendingImage(name string) {
	_, err := os.Stat(name)
	if err != nil {
		m.WithError(err)
		return
	}
	m.pendingAttachImages = append(m.pendingAttachImages, name)
	m.WithMessage("Attached image for next message")
}
