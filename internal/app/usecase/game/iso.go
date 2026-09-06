package gameusecase

import "strings"

func isRawISO(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".iso")
}
