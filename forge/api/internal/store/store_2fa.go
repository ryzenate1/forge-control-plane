package store

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

type TwoFactorSetup struct {
	Secret   string `json:"secret"`
	ImageURL string `json:"image_url"`
}

func generateTOTPSecret() (string, error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw), nil
}

func computeTOTP(secret string, counter uint64) (string, error) {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(secret))
	if err != nil {
		return "", err
	}
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, counter)
	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write(buf)
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	code := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", code%uint32(math.Pow10(6))), nil
}

func verifyTOTP(secret, code string) bool {
	now := time.Now().Unix() / 30
	for _, offset := range []int64{-1, 0, 1} {
		expected, err := computeTOTP(secret, uint64(now+offset))
		if err == nil && hmac.Equal([]byte(expected), []byte(code)) {
			return true
		}
	}
	return false
}

func generateRecoveryTokens() ([]string, error) {
	tokens := make([]string, 10)
	for i := range tokens {
		raw := make([]byte, 10)
		if _, err := rand.Read(raw); err != nil {
			return nil, err
		}
		tokens[i] = fmt.Sprintf("%x", raw)
	}
	return tokens, nil
}

func (s *Store) SetupTwoFactor(ctx context.Context, userID string) (TwoFactorSetup, error) {
	var useTOTP bool
	var email string
	if err := s.db.QueryRow(ctx, `SELECT use_totp, email FROM users WHERE id=$1`, userID).Scan(&useTOTP, &email); err != nil {
		return TwoFactorSetup{}, errors.New("user not found")
	}
	if useTOTP {
		return TwoFactorSetup{}, errors.New("two-factor is already enabled")
	}
	secret, err := generateTOTPSecret()
	if err != nil {
		return TwoFactorSetup{}, errors.New("generate two-factor secret")
	}
	encrypted, err := s.encryptSecret(secret, secretAAD("users", userID, "totp_secret"))
	if err != nil {
		return TwoFactorSetup{}, err
	}
	if _, err := s.db.Exec(ctx, `UPDATE users SET totp_secret='', totp_secret_encrypted=$1 WHERE id=$2`, encrypted, userID); err != nil {
		return TwoFactorSetup{}, errors.New("save two-factor secret")
	}
	return TwoFactorSetup{Secret: secret, ImageURL: fmt.Sprintf("otpauth://totp/ModernGamePanel:%s?secret=%s&issuer=ModernGamePanel", email, secret)}, nil
}

func (s *Store) EnableTwoFactor(ctx context.Context, userID, code, password string) ([]string, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var storedHash, plaintext, encrypted string
	var useTOTP bool
	if err := tx.QueryRow(ctx, `SELECT password_hash, COALESCE(totp_secret,''), COALESCE(totp_secret_encrypted,''), use_totp FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&storedHash, &plaintext, &encrypted, &useTOTP); err != nil {
		return nil, errors.New("user not found")
	}
	if useTOTP {
		return nil, errors.New("two-factor is already enabled")
	}
	secret, err := s.decryptSecret(encrypted, plaintext, secretAAD("users", userID, "totp_secret"))
	if err != nil || secret == "" {
		return nil, errors.New("call setup first to generate a secret")
	}
	if bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(password)) != nil {
		return nil, errors.New("invalid password")
	}
	if !verifyTOTP(secret, code) {
		return nil, errors.New("invalid two-factor code")
	}
	tokens, err := generateRecoveryTokens()
	if err != nil {
		return nil, errors.New("generate recovery tokens")
	}
	hashes := make([]string, len(tokens))
	for i, token := range tokens {
		hash, err := bcrypt.GenerateFromPassword([]byte(token), BcryptCost())
		if err != nil {
			return nil, errors.New("hash recovery token")
		}
		hashes[i] = string(hash)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM recovery_tokens WHERE user_id=$1`, userID); err != nil {
		return nil, err
	}
	for _, hash := range hashes {
		if _, err := tx.Exec(ctx, `INSERT INTO recovery_tokens (id,user_id,token,token_hash) VALUES ($1,$2,'',$3)`, uuid.NewString(), userID, hash); err != nil {
			return nil, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE users SET use_totp=TRUE, totp_authenticated_at=now(), session_version=session_version+1 WHERE id=$1`, userID); err != nil {
		return nil, errors.New("enable two-factor")
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	_ = s.AppendAudit(ctx, &userID, "two-factor enabled", "user", &userID, `{}`)
	return tokens, nil
}

func (s *Store) DisableTwoFactor(ctx context.Context, userID, password string) error {
	var storedHash string
	if err := s.db.QueryRow(ctx, `SELECT password_hash FROM users WHERE id=$1`, userID).Scan(&storedHash); err != nil {
		return errors.New("user not found")
	}
	if bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(password)) != nil {
		return errors.New("invalid password")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE users SET use_totp=FALSE, totp_secret='', totp_secret_encrypted=NULL, totp_authenticated_at=now(), session_version=session_version+1 WHERE id=$1`, userID); err != nil {
		return errors.New("disable two-factor")
	}
	if _, err := tx.Exec(ctx, `DELETE FROM recovery_tokens WHERE user_id=$1`, userID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	_ = s.AppendAudit(ctx, &userID, "two-factor disabled", "user", &userID, `{}`)
	return nil
}

func (s *Store) VerifyTwoFactorCheckpoint(ctx context.Context, userID, code, recoveryToken string) error {
	if recoveryToken != "" {
		tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		rows, err := tx.Query(ctx, `SELECT id::text, COALESCE(token_hash,'') FROM recovery_tokens WHERE user_id=$1 FOR UPDATE`, userID)
		if err != nil {
			return err
		}
		var matchedID string
		for rows.Next() {
			var id, hash string
			if err := rows.Scan(&id, &hash); err != nil {
				rows.Close()
				return err
			}
			if hash != "" && bcrypt.CompareHashAndPassword([]byte(hash), []byte(recoveryToken)) == nil {
				matchedID = id
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		if matchedID == "" {
			return errors.New("invalid recovery token")
		}
		tag, err := tx.Exec(ctx, `DELETE FROM recovery_tokens WHERE id=$1 AND user_id=$2`, matchedID, userID)
		if err != nil || tag.RowsAffected() != 1 {
			return errors.New("recovery token was already consumed")
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		_ = s.AppendAudit(ctx, &userID, "two-factor recovery token used", "user", &userID, `{}`)
		return nil
	}
	var plaintext, encrypted string
	var useTOTP bool
	if err := s.db.QueryRow(ctx, `SELECT COALESCE(totp_secret,''), COALESCE(totp_secret_encrypted,''), use_totp FROM users WHERE id=$1`, userID).Scan(&plaintext, &encrypted, &useTOTP); err != nil {
		return errors.New("user not found")
	}
	if !useTOTP {
		return errors.New("two-factor is not enabled for this user")
	}
	secret, err := s.decryptSecret(encrypted, plaintext, secretAAD("users", userID, "totp_secret"))
	if err != nil || secret == "" {
		return errors.New("two-factor is not configured")
	}
	if !verifyTOTP(secret, code) {
		return errors.New("invalid two-factor code")
	}
	_, _ = s.db.Exec(ctx, `UPDATE users SET totp_authenticated_at=now() WHERE id=$1`, userID)
	return nil
}
