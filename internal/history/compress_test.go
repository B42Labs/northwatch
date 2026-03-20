package history

import (
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
