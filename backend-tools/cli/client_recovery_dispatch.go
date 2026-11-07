package main

// Presence facts are observations from the existing retained native loader,
// never process/admission grants. Each selected provider still validates actual
// held objects, bodies, descriptor/controller and its independent authority.
type recoverySiblingPresence struct {
	prepare, original, birth, recovery, next bool
}

type recoveryIntentRoute uint8

const (
	writerRecoveryRoute recoveryIntentRoute = iota
	localEOFRoute
	localCompleteRoute
)

type recoveryIntentDispatch struct {
	route   recoveryIntentRoute
	local   localReplaceIntent
	nextSHA string
}

func classifyRecoveryDispatch(present bool, body []byte, siblings recoverySiblingPresence, currentSHA string) (recoveryIntentDispatch, error) {
	ordinary := recoveryIntentDispatch{route: writerRecoveryRoute}
	if siblings.next && !siblings.recovery {
		return ordinary, errClientConfigRecovery
	}
	if !present {
		return ordinary, nil
	}
	if incompleteWriterBirth(body) {
		// A schema3 writer may legitimately crash while writing intent AFTER
		// durable prepare+original. Preserve the existing independent preIntent
		// recovery route; never divert it to the no-siblings local EOF provider.
		if siblings.prepare || siblings.original || siblings.birth || siblings.recovery || siblings.next {
			return ordinary, nil
		}
		if !localReplaceEOFPrefix(body, currentSHA) {
			return ordinary, errClientConfigRecovery
		}
		return recoveryIntentDispatch{route: localEOFRoute}, nil
	}
	if record, next, e := parseLocalReplaceIntent(body); e == nil {
		if siblings.prepare || siblings.original || siblings.birth || siblings.recovery || siblings.next {
			return ordinary, errClientConfigRecovery
		}
		return recoveryIntentDispatch{route: localCompleteRoute, local: record, nextSHA: next}, nil
	}
	return ordinary, nil
}
