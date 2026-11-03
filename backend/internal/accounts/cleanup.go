package accounts

import "context"

func (s *Service) cleanAuth(ctx context.Context) {
	_, _ = s.db.Exec(ctx, `DELETE FROM guest_challenges WHERE expires_at<now()-interval '2 minutes'`)
	_, _ = s.db.Exec(ctx, `DELETE FROM relay_browser_tickets WHERE expires_at<now()`)
	_, _ = s.db.Exec(ctx, `DELETE FROM guest_resource_budgets WHERE expires_at<now()`)
	_, _ = s.db.Exec(ctx, `DELETE FROM guest_invites v WHERE v.expires_at<now()-interval '1 day' AND NOT EXISTS(SELECT 1 FROM guest_pair_grants p WHERE p.invite_id=v.id)`)
	_, _ = s.db.Exec(ctx, `DELETE FROM account_rate_limits WHERE expires_at<now()`)
	// Outbox rows belong to a generation; cleaning an older row cannot change
	// delivery state for a later resend. Keep provider ids for delivery diagnosis.
	_, _ = s.db.Exec(ctx, `UPDATE account_mail_outbox o SET status='superseded',payload_cipher='\x'::bytea,lease_until=NULL WHERE o.status IN ('pending','retry') AND EXISTS(SELECT 1 FROM account_email_challenges c WHERE c.id=o.challenge_id AND (c.generation<>o.generation OR c.consumed_at IS NOT NULL OR c.expires_at<now()))`)
	_, _ = s.db.Exec(ctx, `UPDATE account_email_challenges SET pending_password_phc=NULL,reset_hash=NULL WHERE expires_at<now() AND (pending_password_phc IS NOT NULL OR reset_hash IS NOT NULL)`)
	_, _ = s.db.Exec(ctx, `UPDATE account_auth_requests SET result_cipher=NULL,claim_fingerprint=NULL WHERE claimed_until<now() AND result_cipher IS NOT NULL`)
	_, _ = s.db.Exec(ctx, `UPDATE account_auth_requests SET provider_verifier=NULL,provider_state_hash=NULL WHERE expires_at<now() AND provider_verifier IS NOT NULL`)
}
