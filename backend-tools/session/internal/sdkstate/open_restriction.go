package sdkstate

import "errors"

var ErrRestriction = errors.New("SDK state does not satisfy the restrictive startup condition")

type openRestriction uint8

const (
	ordinaryOpen openRestriction = iota
	newServiceOnly
)

// Called by the actual native factory after its fixed FILE_OPEN reply. The
// caller cannot supply fresh; only the original factory's exact missing result
// selects its existing exclusive FILE_CREATE path.
func restrictNativeAdmission(mode openRestriction, fresh bool) error {
	if mode == ordinaryOpen || mode == newServiceOnly && fresh {
		return nil
	}
	return ErrRestriction
}
