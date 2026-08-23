# Encryption at rest and key rotation

Forge encrypts operational secrets with AES-256-GCM. Envelopes have the form
`forge:v1:<key-id>:<base64url nonce+ciphertext>` and are authenticated with AAD
that binds each value to its table, row, and field. Keys are never derived from
`API_AUTH_SECRET`.

## Configuration

- `FORGE_MASTER_KEY`: required whenever `DATABASE_URL` is configured. It must be
  exactly 32 bytes encoded as strict padded base64, or exactly 64 hexadecimal
  characters.
- `FORGE_MASTER_KEY_ID`: active envelope key ID; defaults to `primary`.
- `FORGE_PREVIOUS_MASTER_KEYS`: optional comma-separated
  `key-id=encoded-key` entries used only to decrypt old envelopes.
- `FORGE_ALLOW_EPHEMERAL_MASTER_KEY=true`: development-only explicit escape
  hatch. Forge prints a loud warning because encrypted data becomes
  unrecoverable after process exit. It is rejected in production.

## Migration strategy

Migration `046_encrypt_operational_secrets.sql` adds versioned ciphertext
columns without dropping legacy columns. On every database startup,
`Store.MigrateOperationalSecrets` opens one transaction, locks secret-bearing
rows, writes authenticated ciphertext, hashes legacy recovery codes with
bcrypt, and only then clears reversible plaintext. Any error rolls back the
whole operation, leaving the pre-start state intact. Reruns are idempotent.

Webhook deliveries contain independently encrypted snapshots, so retries keep
the signing secret that existed when the event was queued without storing
plaintext. Mail outbox rows never contain SMTP credentials.

## Rotate the master key

1. Keep the current key available as a previous key.
2. Set a new `FORGE_MASTER_KEY` and a new `FORGE_MASTER_KEY_ID`.
3. Set `FORGE_PREVIOUS_MASTER_KEYS`, for example `old-2026=<old-key>`.
4. Run from `forge/api`:

   ```sh
   DATABASE_URL=... FORGE_MASTER_KEY=... FORGE_MASTER_KEY_ID=new-2027 \
     FORGE_PREVIOUS_MASTER_KEYS=old-2026=... go run ./cmd/api rotate-master-key
   ```

The command transactionally decrypts known envelopes with previous keys and
re-encrypts them with the active key. Normal startup performs the same
idempotent pass. Remove old keys only after this command succeeds and a backup
has been verified.

## Rollback

Before deploying a binary older than migration 046, run:

```sh
DATABASE_URL=... FORGE_MASTER_KEY=... FORGE_PREVIOUS_MASTER_KEYS=... \
  go run ./cmd/api restore-plaintext-secrets
```

This transaction repopulates reversible legacy plaintext columns while
retaining ciphertext for a forward re-deploy. Recovery codes are intentionally
one-way bcrypt hashes and cannot be restored; users must regenerate recovery
codes after a rollback. Do not drop the 046 columns until rollback is no longer
required.
