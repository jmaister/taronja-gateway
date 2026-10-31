package handlers

import "errors"

// errNotImplemented answers operations whose handler is still a stub.
var errNotImplemented = errors.New("not implemented yet")
