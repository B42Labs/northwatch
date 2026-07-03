// Package api implements Northwatch's HTTP server and the JSON response helpers
// shared by every route handler.
//
// It owns the net/http server lifecycle (Server) and the Go 1.22 http.ServeMux
// the handlers register their routes on. The response helpers keep the wire
// format consistent across handlers: WriteJSON / WriteError for the common
// success and error shapes, and WriteJSONList / NonNil so a list response is
// always a JSON array ([]) rather than null. The route handlers themselves live
// in the handler subpackage.
package api
