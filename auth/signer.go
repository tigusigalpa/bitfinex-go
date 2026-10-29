package auth

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/tigusigalpa/bitfinex-go/types"
)

// Signer computes Bitfinex v2 request signatures.
//
// Contract (per official docs):
//
//	signature_payload = "/api/" + apiPath + nonce + raw_body_json
//	signature         = lowercase hex HMAC-SHA384(payload, secret)
//
// The exact JSON bytes that are signed must be the exact bytes
// sent over the wire — the transport serializes once and passes
// the identical byte slice to Sign and to the request body.
type Signer struct {
	creds Credentials
}

// String prevents accidental credential exposure when formatting a signer.
func (s *Signer) String() string { return "Signer{credentials:<redacted>}" }

// GoString redacts debug formatting.
func (s *Signer) GoString() string { return s.String() }

// NewSigner returns a Signer bound to the given credentials.
func NewSigner(c Credentials) *Signer { return &Signer{creds: c} }

// Key returns the API key (needed for the WS auth frame; the
// secret is never exposed).
func (s *Signer) Key() string { return s.creds.key }

// Signature is a detached signature set ready to become headers.
type Signature struct {
	Nonce     string
	Key       string
	Signature string
}

// String redacts detached authentication material.
func (s Signature) String() string { return "Signature{<redacted>}" }

// GoString redacts debug formatting.
func (s Signature) GoString() string { return s.String() }

// MarshalJSON prevents accidental serialization of detached auth material.
func (s Signature) MarshalJSON() ([]byte, error) {
	return nil, &types.ValidationError{Field: "signature", Message: "authentication material is not serializable"}
}

// Sign signs an authenticated REST request.
//
//	apiPath — path relative to /api/ including the version
//	          prefix, e.g. "v2/auth/w/order/submit" (no leading slash)
//	nonce   — numeric nonce string from a NonceProvider
//	rawBody — the exact JSON body bytes being transmitted
func (s *Signer) Sign(apiPath, nonce string, rawBody []byte) (Signature, error) {
	if err := ValidateNonce(nonce); err != nil {
		return Signature{}, err
	}
	if apiPath == "" || strings.HasPrefix(apiPath, "/") {
		return Signature{}, &types.ValidationError{
			Field:   "apiPath",
			Message: "must be a relative path like \"v2/auth/r/wallets\" (no leading slash)",
		}
	}
	payload := "/api/" + apiPath + nonce + string(rawBody)
	return Signature{
		Nonce:     nonce,
		Key:       s.creds.key,
		Signature: hmacSHA384Hex(s.creds.secret, payload),
	}, nil
}

// ValidateNonce checks the numeric range accepted by REST and WebSocket auth.
func ValidateNonce(nonce string) error {
	n, err := strconv.ParseUint(nonce, 10, 64)
	if err != nil || n == 0 || n > MaxNonce || strconv.FormatUint(n, 10) != nonce {
		return &types.NonceError{Message: "provider returned an invalid nonce"}
	}
	return nil
}

// SignWebSocketAuth returns the authSig for the WS auth event:
// HMAC-SHA384 of "AUTH"+nonce. A fresh nonce is required on every
// connect/reconnect — callers must never reuse one.
func (s *Signer) SignWebSocketAuth(nonce string) string {
	return hmacSHA384Hex(s.creds.secret, "AUTH"+nonce)
}

// AuthPayload returns the "AUTH"+nonce payload that must
// accompany the signature.
func AuthPayload(nonce string) string { return "AUTH" + nonce }

func hmacSHA384Hex(secret, payload string) string {
	h := hmac.New(sha512.New384, []byte(secret))
	h.Write([]byte(payload))
	return hex.EncodeToString(h.Sum(nil))
}
