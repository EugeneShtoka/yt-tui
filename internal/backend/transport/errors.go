package transport

import (
	"errors"

	"connectrpc.com/connect"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// rpcErr maps a backend error to the appropriate Connect status code so
// clients can distinguish precondition/not-found failures from genuine
// internal errors, instead of every failure arriving as CodeInternal. (M-1)
func rpcErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, domain.ErrYTNotInitialized) {
		return connect.NewError(connect.CodeFailedPrecondition, err)
	}
	return connect.NewError(connect.CodeInternal, err)
}
