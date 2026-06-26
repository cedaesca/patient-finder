package utils

import (
	"context"
	"log/slog"
)

// GoSafe launches fn in a goroutine with panic recovery. The context passed
// to fn is detached from ctx's cancellation but retains its values (OTel span,
// request_id, slog attrs). That keeps the async work traceable back to the
// caller while outliving the caller's own cancellation — which is what
// fire-and-forget tasks like audit writes need.
//
// If fn panics, the panic is logged with the detached ctx so the log line
// carries the originating trace_id / request_id, then swallowed so the server
// keeps running.
func GoSafe(ctx context.Context, fn func(context.Context)) {
	detached := context.WithoutCancel(ctx)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.ErrorContext(detached, "goroutine panic recovered", "panic", r)
			}
		}()
		fn(detached)
	}()
}
