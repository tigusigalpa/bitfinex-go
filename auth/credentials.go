// Package auth holds the private-API security boundary:
// credentials, the HMAC-SHA384 request signer, and nonce
// provider abstractions. Nothing in this package ever logs or
// serializes a secret.
package auth

import (
	"errors"
	"strings"

	"github.com/tigusigalpa/bitfinex-go/types"
)

// Credentials holds a Bitfinex API key/secret pair. The value is
// copied into the signer; keep the source material out of logs.
type Credentials struct {
	key    string
	secret string
}

// NewCredentials validates and returns a Credentials value.
// Whitespace is rejected rather than trimmed so accidental
// paste artifacts fail loudly.
func NewCredentials(key, secret string) (Credentials, error) {
	if key == "" || secret == "" {
		return Credentials{}, &types.ValidationError{
			Field:   "credentials",
			Message: "API key and secret must both be non-empty",
		}
	}
	if key != strings.TrimSpace(key) || secret != strings.TrimSpace(secret) {
		return Credentials{}, &types.ValidationError{
			Field:   "credentials",
			Message: "API key/secret must not contain leading or trailing whitespace",
		}
	}
	return Credentials{key: key, secret: secret}, nil
}

// Key returns the API key (sent as bfx-apikey).
func (c Credentials) Key() string { return c.key }

// Secret returns the API secret (HMAC key — never transmitted).
func (c Credentials) Secret() string { return c.secret }

// String redacts credentials. Implements fmt.Stringer so a
// Credentials can never leak through %v/%s formatting.
func (c Credentials) String() string { return "Credentials{key:<redacted>}" }

// GoString redacts credentials for %#v formatting.
func (c Credentials) GoString() string { return c.String() }

// MarshalText redacts credentials in any text serialization.
func (c Credentials) MarshalText() ([]byte, error) { return []byte("<redacted>"), nil }

var errRedacted = errors.New("credentials are not serializable")

// MarshalJSON prevents accidental secret serialization.
func (c Credentials) MarshalJSON() ([]byte, error) { return nil, errRedacted }
