package security

import "strings"

func containsAny(src string, targets []string) bool {
	srclow := strings.ToLower(src)
	for _, target := range targets {
		if strings.Contains(srclow, target) {
			return true
		}
	}
	return false
}
