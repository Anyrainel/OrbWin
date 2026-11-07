package main

// Closed checkpoint names; production builds have only no-op callees. The
// opt-in implementation is compiled solely by separately reviewed fixtures.
type recoveryQualificationFault uint32

const (
	recoveryHistoricalJournal recoveryQualificationFault = iota + 1
	recoveryPublishedOriginalRemoved
	recoveryCutNextPrefix
	recoveryNextDurable
	recoveryNextBeforeRename
	recoveryNextPublished
	recoveryHistoricalStagePrepared
	recoveryStageRetired
	recoveryLocalEmpty
	recoveryLocalEarly
	recoveryLocalBase
	recoveryLocalLate
	recoveryWriterIntentPrefix
	recoveryLostMetadataRename
	recoveryLostTargetRename
	recoveryLostStageDisposition
	recoveryLostSnapshotConfigClose
)
