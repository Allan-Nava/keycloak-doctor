package rules

import "fmt"

// humanSeconds renders a Keycloak duration (always seconds) the way an operator
// reads it in the admin console: 300 → "5m", 36000 → "10h", 2592000 → "30d".
func humanSeconds(n int) string {
	switch {
	case n <= 0:
		return "0s"
	case n%(24*3600) == 0:
		return fmt.Sprintf("%dd", n/(24*3600))
	case n%3600 == 0:
		return fmt.Sprintf("%dh", n/3600)
	case n%60 == 0:
		return fmt.Sprintf("%dm", n/60)
	default:
		return fmt.Sprintf("%ds", n)
	}
}
