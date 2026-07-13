package history

import (
	"bytes"
	"compress/gzip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompress_RoundTrip(t *testing.T) {
	original := []byte(`{"name":"sw1","ports":["p1","p2"],"enabled":true}`)
	compressed, err := compress(original)
	require.NoError(t, err)
	assert.NotEqual(t, original, compressed)

	decompressed, err := decompress(compressed)
	require.NoError(t, err)
	assert.Equal(t, original, decompressed)
}

func TestCompress_Empty(t *testing.T) {
	compressed, err := compress([]byte{})
	require.NoError(t, err)

	decompressed, err := decompress(compressed)
	require.NoError(t, err)
	assert.Empty(t, decompressed)
}

func TestDecompress_InvalidData(t *testing.T) {
	_, err := decompress([]byte("not gzip"))
	require.Error(t, err)
}

// TestDecompress_RefusesBomb covers the decompression cap. Snapshot rows arrive
// through the import endpoint, so the compressed bytes are externally
// influenced: a small gzip payload used to expand into an unbounded allocation.
func TestDecompress_RefusesBomb(t *testing.T) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	// Highly compressible: a few hundred KiB on the wire, far past the cap once
	// expanded.
	_, err := w.Write(bytes.Repeat([]byte("A"), maxDecompressedSize+1))
	require.NoError(t, err)
	require.NoError(t, w.Close())

	require.Less(t, buf.Len(), maxDecompressedSize, "the compressed payload must be small; the danger is the expansion")

	_, err = decompress(buf.Bytes())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds")
}

// TestDecompress_AtLimit proves the cap does not reject a row that is exactly at
// the limit, only one past it.
func TestDecompress_AtLimit(t *testing.T) {
	payload := bytes.Repeat([]byte("A"), maxDecompressedSize)
	compressed, err := compress(payload)
	require.NoError(t, err)

	out, err := decompress(compressed)
	require.NoError(t, err)
	assert.Len(t, out, maxDecompressedSize)
}
