package stratum

import "github.com/powerman/rpc-codec/jsonrpc2"

var (
	// Actual returned error may have different message.
	errParse       = jsonrpc2.NewError(-32700, "parse error")
	errRequest     = jsonrpc2.NewError(-32600, "invalid request")
	errMethod      = jsonrpc2.NewError(-32601, "method not found")
	errParams      = jsonrpc2.NewError(-32602, "invalid params")
	errInternal    = jsonrpc2.NewError(-32603, "internal error")
	errServer      = jsonrpc2.NewError(-32000, "server error")
	errServerError = jsonrpc2.NewError(-32001, "jsonrpc2.Error: json.Marshal failed")
)
