package main

import (
	"os"

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
		return nil
	}
	var images [][]byte
	for _, v := range paths {
		b, err := os.ReadFile(v)
		if err != nil {
			return stackerr.Errorf("read image file for attachment: %v", err) // TODO: might be foo.png false positive
		}
		images = append(images, b)

	}
	m.Images = images
	return nil
}
