package main

import "os"

func (m *model) addPendingImage(name string) {
	_, err := os.Stat(name)
	if err != nil {
		m.WithError(err)
		return
	}
	m.pendingAttachImages = append(m.pendingAttachImages, name)
	m.WithTextMessage("Attached image for next message")
}
