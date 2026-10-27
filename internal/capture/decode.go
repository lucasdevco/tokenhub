package capture

import (
	"bufio"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/andybalholm/brotli"
)

// decodeStack closes every decoder, including partially constructed chains.
type decodeStack struct {
	io.Reader
	closers []io.Closer
}

var errUnsupportedEncoding = errors.New("unsupported content encoding")

func (d *decodeStack) Close() error {
	var err error
	for i := len(d.closers) - 1; i >= 0; i-- {
		err = errors.Join(err, d.closers[i].Close())
	}
	return err
}

func decompressReader(source io.Reader, encoding string) (io.ReadCloser, error) {
	stack := &decodeStack{Reader: source}
	encodings := strings.Split(strings.ToLower(encoding), ",")
	// Validate the entire chain before reading so unsupported encodings can be
	// saved verbatim without losing a partially consumed compressed prefix.
	for _, name := range encodings {
		switch strings.TrimSpace(name) {
		case "", "identity", "gzip", "x-gzip", "deflate", "br":
		default:
			return nil, fmt.Errorf("%w: %q", errUnsupportedEncoding, name)
		}
	}
	for i := len(encodings) - 1; i >= 0; i-- {
		var next io.ReadCloser
		var err error
		switch name := strings.TrimSpace(encodings[i]); name {
		case "", "identity":
			continue
		case "gzip", "x-gzip":
			next, err = gzip.NewReader(stack.Reader)
		case "deflate":
			// HTTP deflate is zlib-wrapped; also accept servers emitting raw DEFLATE.
			buffered := bufio.NewReader(stack.Reader)
			header, _ := buffered.Peek(2)
			if len(header) == 2 && header[0]&15 == 8 && (int(header[0])*256+int(header[1]))%31 == 0 {
				next, err = zlib.NewReader(buffered)
			} else {
				next = flate.NewReader(buffered)
			}
		case "br":
			next = io.NopCloser(brotli.NewReader(stack.Reader))
		}
		if err != nil {
			stack.Close()
			return nil, err
		}
		stack.Reader = next
		stack.closers = append(stack.closers, next)
	}
	return stack, nil
}
