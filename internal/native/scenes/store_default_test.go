package scenes

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestNewFileStore_PathSuffix(t *testing.T) {
	s, err := NewFileStore()
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	p := filepath.ToSlash(s.path)
	if !strings.HasSuffix(p, "/sonos-playlist/scenes.json") {
		t.Fatalf("unexpected path: %q", p)
	}
}
