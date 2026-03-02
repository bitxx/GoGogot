package transport

import "context"

type ctxKey int

const (
	ctxKeyTransport ctxKey = iota
	ctxKeyChannelID
)

func WithTransport(ctx context.Context, t Transport, channelID string) context.Context {
	ctx = context.WithValue(ctx, ctxKeyTransport, t)
	return context.WithValue(ctx, ctxKeyChannelID, channelID)
}

func FromContext(ctx context.Context) (Transport, string, bool) {
	t, ok1 := ctx.Value(ctxKeyTransport).(Transport)
	ch, ok2 := ctx.Value(ctxKeyChannelID).(string)
	return t, ch, ok1 && ok2
}
