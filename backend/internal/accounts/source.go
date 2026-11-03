package accounts

import "net/netip"

// The edge overwrites this header; arbitrary direct clients cannot select a
// source bucket. No forwarded header is trusted unless the peer is configured.
func (s *Service) ClientIP(peer netip.Addr, forwarded string) string {
	peer = peer.Unmap()
	for _, prefix := range s.config.TrustedProxies {
		if prefix.Contains(peer) {
			if source, err := netip.ParseAddr(forwarded); err == nil {
				return source.Unmap().String()
			}
		}
	}
	return peer.String()
}
