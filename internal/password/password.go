// Package password holds the rule shared by every route that hashes a new password.
package password

import "errors"

// MaxBytes is the longest input bcrypt hashes; it reads the password as UTF-8 bytes, not characters.
const MaxBytes = 72

var ErrTooLong = errors.New("password must be at most 72 bytes (accented characters count for 2)")

func CheckLength(pw string) error {
	if len(pw) > MaxBytes {
		return ErrTooLong
	}
	return nil
}
