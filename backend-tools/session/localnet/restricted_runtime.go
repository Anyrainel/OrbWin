package localnet

import (
	"context"
	"encoding/json"
)

func (runtime *ServiceRuntime) checkExpectedServer(expected *ExpectedServer) error {
	if runtime == nil || runtime.opts.SelfLease == nil {
		return errRestricted
	}
	actual, err := runtime.opts.SelfLease.Identity()
	if err != nil || !actual.Valid() {
		return errRestricted
	}
	return compareExpectedServer(expected, actual.PID, actual.CreationTime)
}

func (runtime *ServiceRuntime) configureExpected(ctx context.Context, revision uint64, settings ServiceSettings, expected *ExpectedServer) (SettingsSnapshot, error) {
	encoded, err := json.Marshal(settings)
	if err != nil {
		return SettingsSnapshot{}, err
	}
	var copied ServiceSettings
	if err = json.Unmarshal(encoded, &copied); err != nil {
		return SettingsSnapshot{}, err
	}
	var comparison *ExpectedServer
	if expected != nil {
		value := *expected
		comparison = &value
	}
	reply, err := runtime.request(ctx, runtimeCommand{kind: "configure", revision: revision, settings: copied, expectedServer: comparison})
	return reply.settings, err
}
