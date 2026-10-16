package storage

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func onePixelPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 20, G: 40, B: 60, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
func TestSaveWritesGeneratedSafeName(t *testing.T) {
	dir := t.TempDir()
	s := &Local{Dir: dir, MaxBytes: 1024}
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	name, err := s.Save(bytes.NewReader(onePixelPNG(t)))
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[a-f0-9]{32}\.png$`).MatchString(name) {
		t.Fatalf("unsafe generated file name: %q", name)
	}
	if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
		t.Fatalf("saved file does not exist: %v", err)
	}
	f, typ, err := s.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if typ != "image/png" {
		t.Fatalf("unexpected content type %q", typ)
	}
	if err := s.Remove(name); err != nil {
		t.Fatal(err)
	}
}
func TestSaveRejectsNonImage(t *testing.T) {
	s := &Local{Dir: t.TempDir(), MaxBytes: 1024}
	_ = s.Init()
	if _, err := s.Save(bytes.NewBufferString("hello, world")); err == nil {
		t.Fatal("expected non-image rejection")
	}
}
func TestSaveRejectsOversize(t *testing.T) {
	s := &Local{Dir: t.TempDir(), MaxBytes: 5}
	_ = s.Init()
	if _, err := s.Save(bytes.NewReader(onePixelPNG(t))); err == nil {
		t.Fatal("expected oversized image rejection")
	}
}
func TestRemoveRejectsPathTraversal(t *testing.T) {
	s := &Local{Dir: t.TempDir(), MaxBytes: 1024}
	if err := s.Remove("../../etc/passwd"); err == nil {
		t.Fatal("expected invalid file name rejection")
	}
}
