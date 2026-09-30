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
	// Accept standardized UUID versions used by imported and current records,
	// while rejecting nil, unversioned, and non-RFC variants.
	if value == "00000000-0000-0000-0000-000000000000" ||
		(value[14] < '1' || value[14] > '5') ||
		(value[19] != '8' && value[19] != '9' && value[19] != 'a' && value[19] != 'A' && value[19] != 'b' && value[19] != 'B') {
		return errInvalid
	}
	return nil
}

func IsValid(value string) bool {
	return Validate(value) == nil
}
