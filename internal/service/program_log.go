package service

import (
	"io"
	"log"
	"os"
	"path/filepath"
)

// OpenProgramLog keeps diagnostics beside the config, separate from traffic captures.
// The caller must close the file after all services have stopped.
func OpenProgramLog(configFile string) (*os.File, error) {
	filename, err := filepath.Abs(filepath.Join(filepath.Dir(configFile), "app.log"))
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, err
	}
	log.SetOutput(io.MultiWriter(os.Stderr, file))
	log.Printf("TokenHub starting; program log: %s", filename)
	return file, nil
}
