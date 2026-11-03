// Package emailaddr is the one form an email address is compared and stored in; it owns no table.
package emailaddr

import (
	"encoding/json"
	"strings"
)

// Normalize trims and lowercases an address.
func Normalize(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// Input is a request field normalised while decoding, so validation sees the trimmed address.
type Input string

func (e *Input) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	*e = Input(Normalize(s))
	return nil
}
