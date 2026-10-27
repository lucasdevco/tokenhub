package capture

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"io"
	"testing"
)

func TestDecodeWrappedRawAndStackedEncoding(t *testing.T) {
	payload := []byte(`{"message":"complete"}`)
	for _, kind := range []string{"zlib", "raw", "stacked"} {
		t.Run(kind, func(t *testing.T) {
			var encoded bytes.Buffer
			var writer io.WriteCloser
			encoding := "deflate"
			switch kind {
			case "raw":
				writer, _ = flate.NewWriter(&encoded, flate.DefaultCompression)
			case "zlib":
				writer = zlib.NewWriter(&encoded)
			case "stacked":
				writer = gzip.NewWriter(&encoded)
			}
			writer.Write(payload)
			writer.Close()
			if kind == "stacked" {
				var outer bytes.Buffer
				w := zlib.NewWriter(&outer)
				w.Write(encoded.Bytes())
				w.Close()
				encoded = outer
				encoding = "gzip, deflate"
			}
			reader, err := decompressReader(&encoded, encoding)
			if err != nil {
				t.Fatal(err)
			}
			got, err := io.ReadAll(reader)
			closeErr := reader.Close()
			if err != nil || closeErr != nil || !bytes.Equal(got, payload) {
				t.Fatalf("decode: %q %v %v", got, err, closeErr)
			}
		})
	}
}
