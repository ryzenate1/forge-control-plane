//go:build webauthn

package auth

import (
	"github.com/go-webauthn/webauthn/webauthn"
)

type WebAuthn struct {
	rpID          string
	rpOrigin      string
	rpDisplayName string
	webauthn      *webauthn.WebAuthn
}

func NewWebAuthn(rpID, rpOrigin, rpDisplayName string) (*WebAuthn, error) {
	wconfig := &webauthn.Config{
		RPDisplayName: rpDisplayName,
		RPID:          rpID,
		RPOrigin:      rpOrigin,
	}

	webauthn, err := webauthn.New(wconfig)
	if err != nil {
		return nil, err
	}

	return &WebAuthn{
		rpID:          rpID,
		rpOrigin:      rpOrigin,
		rpDisplayName: rpDisplayName,
		webauthn:      webauthn,
	}, nil
}

func (w *WebAuthn) BeginRegistration(userID string) (webauthn.CredentialCreation, []byte, error) {
	user := webauthn.User{
		ID:          []byte(userID),
		Name:        userID,
		DisplayName: userID,
	}

	cc, sessionData, err := w.webauthn.BeginRegistration(user)
	if err != nil {
		return webauthn.CredentialCreation{}, nil, err
	}

	return cc, sessionData, nil
}

func (w *WebAuthn) FinishRegistration(userID string, cred webauthn.Credential, sessionData []byte) error {
	user := webauthn.User{
		ID:          []byte(userID),
		Name:        userID,
		DisplayName: userID,
	}

	_, err := w.webauthn.FinishRegistration(user, sessionData, cred)
	return err
}
