package main

// This is the owner's local config-replace transaction, not a Docker writer.
// Its producer never loans the file or starts a child. There is no durable
// original to restore: explicit recovery keeps validated current settings.
type localReplaceIntent struct {
	Schema  uint8  `json:"schema"`
	Kind    string `json:"kind"`
	BaseSHA string `json:"baseSha256"`
}

// The Go producer has exactly three fields; Rust also binds the intended new
// bytes. Keep these closed shapes distinct from schema3 writer admission.
func parseLocalReplaceIntent(body []byte) (localReplaceIntent, string, error) {
	var base localReplaceIntent
	if closedWriterRecord(body, 4096, 3, &base) == nil {
		if base.Schema == 1 && base.Kind == "config-replace" && lowerDigest(base.BaseSHA) {
			return base, "", nil
		}
		return localReplaceIntent{}, "", errClientConfigRecovery
	}
	// A flat struct is intentional: closedWriterRecord checks exact JSON fields
	// and types before decoding, including rejection of null and case aliases.
	var full struct {
		Schema  uint8  `json:"schema"`
		Kind    string `json:"kind"`
		BaseSHA string `json:"baseSha256"`
		NextSHA string `json:"nextSha256"`
	}
	if closedWriterRecord(body, 4096, 4, &full) != nil || full.Schema != 1 || full.Kind != "config-replace" || !lowerDigest(full.BaseSHA) || !lowerDigest(full.NextSHA) {
		return localReplaceIntent{}, "", errClientConfigRecovery
	}
	return localReplaceIntent{full.Schema, full.Kind, full.BaseSHA}, full.NextSHA, nil
}

func localReplaceCurrentMatches(record localReplaceIntent, next, current string) bool {
	return lowerDigest(current) && (next == "" || current == record.BaseSHA || current == next)
}
