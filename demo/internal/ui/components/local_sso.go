package components

import (
	"strings"

	"github.com/nkhine/gohtmxelm/demo/internal/localsso"
)

func ssoGroups(claims localsso.Claims) string {
	if len(claims.Groups) == 0 {
		return "none"
	}
	return strings.Join(claims.Groups, ", ")
}

func ssoAuthTime(claims localsso.Claims) string {
	if claims.AuthTime.IsZero() {
		return "not signed in"
	}
	return claims.AuthTime.Format("15:04:05 MST")
}
