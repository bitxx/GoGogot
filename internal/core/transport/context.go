package transport

import "context"

type replierKey struct{}

func WithReplier(ctx context.Context, r Replier) context.Context {
	return context.WithValue(ctx, replierKey{}, r)
}

func ReplierFromContext(ctx context.Context) (Replier, bool) {
	r, ok := ctx.Value(replierKey{}).(Replier)
	return r, ok
}

type busKey struct{}

func WithBus(ctx context.Context, b *Bus) context.Context {
	return context.WithValue(ctx, busKey{}, b)
}

func BusFromContext(ctx context.Context) (*Bus, bool) {
	b, ok := ctx.Value(busKey{}).(*Bus)
	return b, ok
}
