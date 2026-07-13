package history

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
)

func compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		return nil, fmt.Errorf("gzip write: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("gzip close: %w", err)
	}
	return buf.Bytes(), nil
}

// maxDecompressedSize bounds one decompressed snapshot row at 10 MiB, matching
// the audit store's limit. Snapshot rows can arrive through the import endpoint,
// so the compressed bytes are externally influenced: without a limit a small
// gzip payload expands into an arbitrarily large allocation.
const maxDecompressedSize = 10 << 20

func decompress(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("gzip reader: %w", err)
	}
	defer func() { _ = r.Close() }()

	// Read one byte past the limit so hitting it is distinguishable from a row
	// that is exactly maxDecompressedSize long, and fail loudly rather than
	// silently returning truncated (and therefore unparseable) JSON.
	out, err := io.ReadAll(io.LimitReader(r, maxDecompressedSize+1))
	if err != nil {
		return nil, fmt.Errorf("gzip read: %w", err)
	}
	if len(out) > maxDecompressedSize {
		return nil, fmt.Errorf("gzip read: decompressed row exceeds %d bytes", maxDecompressedSize)
	}
	return out, nil
}
