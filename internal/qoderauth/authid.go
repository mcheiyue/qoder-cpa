package qoderauth

import "crypto/sha256"

// newAuthID computes "qoder-<hex>" from a user ID string.
func newAuthID(userID string) AuthID {
	h := sha256.Sum256([]byte(userID))
	return AuthID("qoder-" + hexEncode(h[:]))
}

// AuthIDForUser returns the stable CPA auth identifier for a Qoder user.
func AuthIDForUser(userID string) AuthID { return newAuthID(userID) }

// AuthIDForProfile derives the auth identifier for a credential profile.
// Intl profiles keep the historical userID-only derivation so existing IDs
// never drift; CN mixes a "cn\0" seed so the same userID cannot collide
// across regions.
func AuthIDForProfile(userID string, profile TransportProfile) AuthID {
	if profile == TransportProfileCosyCN {
		h := sha256.Sum256([]byte("cn\x00" + userID))
		return AuthID("qoder-" + hexEncode(h[:]))
	}
	return AuthIDForUser(userID)
}

// hexEncode returns a lowercase hex string.
func hexEncode(b []byte) string {
	const hex = "0123456789abcdef"
	s := make([]byte, len(b)*2)
	for i, v := range b {
		s[i*2] = hex[v>>4]
		s[i*2+1] = hex[v&0x0f]
	}
	return string(s)
}
