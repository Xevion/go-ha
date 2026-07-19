package internal

import (
	"bufio"
	"compress/flate"
	"compress/zlib"
	"io"
)

// deflateReader decompresses a Content-Encoding: deflate body, closing the
// decompressor and the response body together.
type deflateReader struct {
	dec io.ReadCloser
	src io.Closer
}

func (d *deflateReader) Read(p []byte) (int, error) { return d.dec.Read(p) }

func (d *deflateReader) Close() error {
	err := d.dec.Close()
	if cerr := d.src.Close(); err == nil {
		err = cerr
	}
	return err
}

// decompressDeflate reads a deflate body under either framing.
//
// RFC 9110 defines the deflate coding as zlib-wrapped, and Home Assistant sends
// it that way, but resty decodes it as a raw deflate stream. That mismatch made
// every compressed response decode to nothing: resty discards the read error,
// so the body arrived empty with no error to notice. Responses small enough to
// go uncompressed were unaffected, which is why only large ones broke.
//
// The framing is sniffed rather than assumed, because senders of raw deflate
// exist too, and the header is what distinguishes them.
func decompressDeflate(r io.ReadCloser) (io.ReadCloser, error) {
	br := bufio.NewReader(r)

	if header, err := br.Peek(2); err == nil && isZlibHeader(header) {
		dec, err := zlib.NewReader(br)
		if err != nil {
			return nil, err
		}
		return &deflateReader{dec: dec, src: r}, nil
	}

	return &deflateReader{dec: flate.NewReader(br), src: r}, nil
}

// isZlibHeader reports whether the two bytes are a zlib header: deflate
// compression, and a check value that makes the pair a multiple of 31.
func isZlibHeader(b []byte) bool {
	if len(b) < 2 || b[0]&0x0f != 8 {
		return false
	}
	return (uint16(b[0])<<8|uint16(b[1]))%31 == 0
}
