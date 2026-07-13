package handler

import (
	"context"

	"github.com/ovn-kubernetes/libovsdb/client"
)

// errListClient is a client.Client whose cache reads always fail, so handlers'
// 500 paths can be exercised. A real client cannot stand in here: List and
// WhereCache serve from the in-process cache, which keeps answering even after
// the connection drops.
//
// Only the cache-read methods are implemented; the embedded nil interface makes
// any other call panic, which is the intent — a handler under test must not
// reach for anything else.
type errListClient struct {
	client.Client
	err error
}

func (c errListClient) List(_ context.Context, _ any) error { return c.err }

func (c errListClient) WhereCache(_ any) client.ConditionalAPI {
	return errConditionalAPI{err: c.err}
}

type errConditionalAPI struct {
	client.ConditionalAPI
	err error
}

func (c errConditionalAPI) List(_ context.Context, _ any) error { return c.err }
