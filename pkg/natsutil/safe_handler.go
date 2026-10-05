package natsutil

import (
	"log/slog"
	"runtime/debug"

	"github.com/nats-io/nats.go"
)

// SafeHandler wraps a NATS message handler with a panic recovery mechanism.
func SafeHandler(handler nats.MsgHandler) nats.MsgHandler {
	return func(msg *nats.Msg) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("Panic in NATS handler", "panic", r, "stack", string(debug.Stack()))
				msg.Nak()
			}
		}()
		handler(msg)
	}
}
