package service

import "context"

func (s *Service) relayOfferSessionAlive(ctx context.Context, offer RelayOffer) bool {
	if s.accounts == nil {
		return false
	}
	if offer.GuestSessionID != "" {
		return s.accounts.GuestSessionAlive(ctx, offer.GuestSessionID) == nil
	}
	return s.accounts.SessionAlive(ctx, offer.AuthSessionID) == nil
}
