package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/b42labs/northwatch/internal/api"
)

const (
	// maxBodySize caps an ordinary JSON request body at 1 MiB. Every body-decoding
	// handler goes through it: without a cap, a request body is read fully into
	// memory and any unauthenticated client can drive the process out of it.
	maxBodySize = 1 << 20

	// maxImportBodySize caps a snapshot import at 100 MiB. Imports carry a whole
	// database copy, so they need far more headroom than an ordinary request —
	// but a deliberate limit rather than none.
	maxImportBodySize = 100 << 20
)

// decodeJSONBody reads at most maxBytes from the request body and decodes it
// into dst. It reports whether decoding succeeded; on failure it has already
// written the response — 413 for an oversized body, 400 for malformed JSON — so
// the caller just returns.
//
// When allowEmpty is set, an absent body leaves dst at its zero value and
// succeeds, which is what handlers with an entirely optional body want. Note
// that this reads the body rather than testing ContentLength: a chunked request
// reports a length of -1, and skipping the decode on that basis silently drops
// the fields the client sent.
func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64, allowEmpty bool) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)

	err := json.NewDecoder(r.Body).Decode(dst)
	switch {
	case err == nil:
		return true
	case allowEmpty && errors.Is(err, io.EOF):
		return true
	}

	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		api.WriteError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return false
	}
	api.WriteError(w, http.StatusBadRequest, "invalid JSON body")
	return false
}
