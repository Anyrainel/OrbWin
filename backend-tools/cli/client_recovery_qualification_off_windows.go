//go:build windows && !orbwin_recovery_qualification

package main

func recoveryQualificationCheckpoint(*nativeClientConfig, recoveryQualificationFault) {}
func recoveryQualificationPrefix(*nativePrivateRecord, []byte) error                  { return nil }
func recoveryQualificationLost(*nativeClientConfig, recoveryQualificationFault) bool  { return false }
func observeQualificationDeletePending(*configData)                                   {}
func recoveryQualificationBytes(_ *nativeClientConfig, _ writerRecoveryRecord, normal []byte) ([]byte, error) {
	return normal, nil
}
