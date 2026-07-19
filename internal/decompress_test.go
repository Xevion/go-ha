package internal

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func zlibFramed(t *testing.T, payload string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	_, err := w.Write([]byte(payload))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return buf.Bytes()
}

func rawFramed(t *testing.T, payload string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, flate.DefaultCompression)
	require.NoError(t, err)
	_, err = w.Write([]byte(payload))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return buf.Bytes()
}

// Home Assistant compresses responses past a size threshold and sends the
// zlib-framed deflate that RFC 9110 specifies. Decoding that as a raw deflate
// stream yields nothing, and because the read error is discarded upstream it
// surfaces as an empty body rather than a failure.
func TestClientDecodesZlibFramedDeflate(t *testing.T) {
	body := `[{"entity_id":"light.kitchen","state":"on"}]`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", "deflate")
		_, _ = w.Write(zlibFramed(t, body))
	}))
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	require.NoError(t, err)

	got, err := NewHttpClient(context.Background(), u, "token").GetStates()
	require.NoError(t, err)
	assert.Equal(t, body, string(got))
}

// Raw deflate is out of spec but senders of it exist, and the framings are
// distinguishable by their header, so both are accepted.
func TestClientDecodesRawDeflate(t *testing.T) {
	body := `[{"entity_id":"light.kitchen","state":"on"}]`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", "deflate")
		_, _ = w.Write(rawFramed(t, body))
	}))
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	require.NoError(t, err)

	got, err := NewHttpClient(context.Background(), u, "token").GetStates()
	require.NoError(t, err)
	assert.Equal(t, body, string(got))
}

// A success status with nothing in it is not a valid snapshot. Passing it on
// left the caller decoding an empty document and reporting a JSON syntax error,
// which describes neither the cause nor the layer it came from.
func TestEmptySuccessBodyIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	require.NoError(t, err)
	c := NewHttpClient(context.Background(), u, "token")

	_, err = c.GetStates()
	assert.ErrorIs(t, err, ErrEmptyResponse)

	_, err = c.GetState("light.kitchen")
	assert.ErrorIs(t, err, ErrEmptyResponse)
}

func TestZlibHeaderDetection(t *testing.T) {
	assert.True(t, isZlibHeader([]byte{0x78, 0x9c}), "default compression")
	assert.True(t, isZlibHeader([]byte{0x78, 0x5e}), "what Home Assistant sends")
	assert.True(t, isZlibHeader([]byte{0x78, 0x01}), "no compression")
	assert.True(t, isZlibHeader([]byte{0x78, 0xda}), "best compression")

	assert.False(t, isZlibHeader([]byte{0x78}), "too short")
	assert.False(t, isZlibHeader(nil), "empty")
	// A raw deflate stream of ASCII text usually opens with a block header whose
	// low nibble is not 8.
	assert.False(t, isZlibHeader([]byte{0x4b, 0x4c}), "raw deflate")
}
