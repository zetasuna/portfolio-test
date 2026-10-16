package storage

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var safeName = regexp.MustCompile(`^[a-f0-9]{32}\.(jpg|png)$`)
var ErrInvalidImage = errors.New("only valid JPEG and PNG images are accepted")

type Local struct {
	Dir      string
	MaxBytes int64
}

func (s *Local) Init() error { return os.MkdirAll(s.Dir, 0750) }
func (s *Local) Save(r io.Reader) (string, error) {
	data, err := io.ReadAll(io.LimitReader(r, s.MaxBytes+1))
	if err != nil {
		return "", err
	}
	if int64(len(data)) > s.MaxBytes {
		return "", fmt.Errorf("image exceeds %d bytes", s.MaxBytes)
	}
	if len(data) == 0 {
		return "", ErrInvalidImage
	}
	kind := http.DetectContentType(data)
	ext := ""
	switch kind {
	case "image/jpeg":
		ext = ".jpg"
	case "image/png":
		ext = ".png"
	default:
		return "", ErrInvalidImage
	}
	// DecodeConfig verifies the image structure rather than trusting the file name or Content-Type.
	cfg, format, decodeErr := imageConfig(data)
	if decodeErr != nil || format != strings.TrimPrefix(ext, ".") && !(ext == ".jpg" && format == "jpeg") {
		return "", ErrInvalidImage
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 10000 || cfg.Height > 10000 || int64(cfg.Width)*int64(cfg.Height) > 12000000 {
		return "", fmt.Errorf("image dimensions exceed allowed limits")
	}
	if _, actualFormat, err := image.Decode(bytes.NewReader(data)); err != nil || actualFormat != format {
		return "", ErrInvalidImage
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	name := hex.EncodeToString(b) + ext
	tmp, err := os.CreateTemp(s.Dir, ".upload-*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err = tmp.Write(data); err != nil {
		tmp.Close()
		return "", err
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return "", err
	}
	if err = tmp.Close(); err != nil {
		return "", err
	}
	if err = os.Chmod(tmpName, 0640); err != nil {
		return "", err
	}
	if err = os.Rename(tmpName, filepath.Join(s.Dir, name)); err != nil {
		return "", err
	}
	return name, nil
}
func imageConfig(data []byte) (image.Config, string, error) {
	return image.DecodeConfig(bytes.NewReader(data))
}
func (s *Local) Remove(name string) error {
	if name == "" {
		return nil
	}
	if filepath.Base(name) != name || !safeName.MatchString(name) {
		return errors.New("invalid stored file name")
	}
	err := os.Remove(filepath.Join(s.Dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
func (s *Local) Open(name string) (*os.File, string, error) {
	if filepath.Base(name) != name || !safeName.MatchString(name) {
		return nil, "", os.ErrNotExist
	}
	f, err := os.Open(filepath.Join(s.Dir, name))
	if err != nil {
		return nil, "", err
	}
	typ := mime.TypeByExtension(filepath.Ext(name))
	if typ == "" {
		typ = "application/octet-stream"
	}
	return f, typ, nil
}
