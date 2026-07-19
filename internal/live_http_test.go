//go:build live

package internal

import (
	"context"
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Home Assistant compresses responses past a size threshold, so the snapshot
// endpoint arrives compressed on any real instance while a single entity does
// not. That difference is the whole bug: the uncompressed path always worked,
// which is why a fake serving small bodies could not surface it.
func TestLiveCompressedSnapshotDecodes(t *testing.T) {
	base, token := os.Getenv("HA_URL"), os.Getenv("HA_TOKEN")
	if base == "" || token == "" {
		t.Skip("HA_URL and HA_TOKEN required for the live suite")
	}

	u, err := url.Parse(base)
	require.NoError(t, err)
	c := NewHttpClient(context.Background(), u, token)

	one, err := c.GetState("sun.sun")
	require.NoError(t, err)
	assert.NotEmpty(t, one, "a single entity is small enough to arrive uncompressed")

	all, err := c.GetStates()
	require.NoError(t, err)
	assert.Greater(t, len(all), len(one),
		"the full snapshot is compressed on the wire and has to survive decoding")
}
