package sdkstate

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var uuid = regexp.MustCompile(`^(?:[a-f0-9]{32}|[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12})$`)

func installationID(s string) bool { return uuid.MatchString(s) && strings.Trim(s, "0-") != "" }
func validDaemon(s string) bool {
	return len(s) > 0 && len(s) <= 256 && utf8.ValidString(s) && !strings.ContainsFunc(s, unicode.IsControl)
}

var namespaceName = regexp.MustCompile(`^namespace-[a-f0-9]{32}\.jsonl$`)
var lanReviewName = regexp.MustCompile(`^lan-review-[a-f0-9]{32}\.json$`)
var relayName = regexp.MustCompile(`^relay-[a-f0-9]{32}\.receipt$`)
var imageName = regexp.MustCompile(`^tools-image-[a-f0-9]{64}(?:-[a-f0-9]{32}\.archived-receipt|\.receipt)$`)
var windowsName = regexp.MustCompile(`^windows-trust-[a-f0-9]{64}\.json$`)
var wslName = regexp.MustCompile(`^wsl-trust-[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}-[a-f0-9]{64}\.json$`)
var rotationName = regexp.MustCompile(`^\.rotation-[a-f0-9]{64}\.(?:old|new)$`)
var archiveName = regexp.MustCompile(`^authority-(?:retired|abandoned)-[a-f0-9]{64}\.pem$`)

func recordBudget(n string) int64 {
	switch n {
	case "authority-v1.json", "settings-v1.json":
		return 128 << 10
	case "authority-rotation-v1.json":
		return 384 << 10
	case "lan-identity.json":
		return 2048
	}
	switch {
	case lanReviewName.MatchString(n):
		return 64 << 10
	case namespaceName.MatchString(n):
		return 8192
	case relayName.MatchString(n):
		return 128 << 10
	case imageName.MatchString(n):
		return 4096
	case windowsName.MatchString(n) || wslName.MatchString(n):
		return 32768
	case rotationName.MatchString(n) || archiveName.MatchString(n):
		return 128 << 10
	}
	return 0
}
