package serverid

import "errors"

var errInvalid = errors.New("server id must be a canonical UUID")

// Validate rejects identifiers that could escape a server root. Forge server
// identifiers are canonical UUIDs, so Beacon accepts only the 8-4-4-4-12 UUID
// representation (case-insensitive hexadecimal digits).
func Validate(value string) error {
	if len(value) != 36 {
		return errInvalid
	}
	for index := 0; index < len(value); index++ {
		switch index {
		case 8, 13, 18, 23:
			if value[index] != '-' {
				return errInvalid
			}
		default:
			char := value[index]
			if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
				return errInvalid
			}
		}
	}
	return nil
}

func IsValid(value string) bool {
	return Validate(value) == nil
}
