ALTER TABLE onboarding_tokens
    DROP CONSTRAINT IF EXISTS onboarding_tokens_state_check;

ALTER TABLE onboarding_tokens
    ADD CONSTRAINT onboarding_tokens_state_check
    CHECK (state IN ('pending', 'approved', 'rejected', 'revoked', 'expired', 'consumed'));
