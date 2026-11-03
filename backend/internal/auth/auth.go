package auth

import "context"

type Identity struct {
	OwnerID        string
	DeviceID       string
	SessionID      string
	GuestSessionID string
	ResourceIP     string
	RelayFromBytes *int64
}

func (i Identity) HasSession() bool { return i.SessionID != "" || i.GuestSessionID != "" }

type contextKey struct{}

func WithIdentity(ctx context.Context, identity Identity) context.Context {
	return context.WithValue(ctx, contextKey{}, identity)
}

func FromContext(ctx context.Context) (Identity, bool) {
	identity, ok := ctx.Value(contextKey{}).(Identity)
	return identity, ok
}
