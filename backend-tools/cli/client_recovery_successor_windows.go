//go:build windows

package main

import (
	"bytes"
	"unsafe"

	"golang.org/x/sys/windows"
)

func (n *nativeClientConfig) successorFence() error {
	n.successorUnknown = true
	n.poison = true
	retainedConfigGuards.fence()
	return errClientConfigRecovery
}
func successorNumbers(d *configData) (uint32, uint32, uint32, string, error) {
	if d == nil || d.check() != nil {
		return 0, 0, 0, "", errClientConfigRecovery
	}
	var id windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(windows.Handle(d.file.Fd()), &id) != nil {
		return 0, 0, 0, "", errClientConfigRecovery
	}
	sd, e := configDescriptorSHA(windows.Handle(d.file.Fd()))
	return id.VolumeSerialNumber, id.FileIndexHigh, id.FileIndexLow, sd, e
}
func (n *nativeClientConfig) successorWitness(old writerRecoveryRecord, oldFile *configData, oldBody []byte, current *configData, currentBody []byte, stage *configData, stageBody []byte, selector string, target []byte) (writerRecoveryV2, error) {
	v := recoveryV2From(old, selector)
	v.Schema = 2
	v.CurrentSHA = digestBytes(currentBody)
	v.RestoreSHA = digestBytes(target)
	var e error
	v.CurrentVolume, v.CurrentHigh, v.CurrentLow, v.CurrentSD, e = successorNumbers(current)
	if e != nil {
		return v, e
	}
	if oldFile != nil {
		v.PreviousVolume, v.PreviousHigh, v.PreviousLow, v.PreviousSD, e = successorNumbers(oldFile)
		if e != nil {
			return v, e
		}
		v.PreviousSHA = digestBytes(oldBody)
		v.PreviousRestoreSHA = old.RestoreSHA
	}
	if stage != nil {
		v.StagePresent = true
		v.StageVolume, v.StageHigh, v.StageLow, v.StageSD, e = successorNumbers(stage)
		if e != nil {
			return v, e
		}
		v.StageSHA = digestBytes(stageBody)
	}
	return v, nil
}

func validateNextBody(body []byte, expected writerRecoveryV2, baseline []byte, profile string) (partial bool, resultErr error) {
	if incompleteRecoveryRecord(body) {
		if !recoveryNextPrefix(body, expected, profile) {
			return false, errClientConfigRecovery
		}
		return true, nil
	}
	_, v, e := parseRecoveryAny(body)
	if e != nil || v == nil || !sameRecoveryV2Authority(*v, expected) || !freshSelector(v.TargetSelector, profile) {
		return false, errClientConfigRecovery
	}
	target, e := recoveryTarget(baseline, v.TargetSelector, profile)
	if e != nil || digestBytes(target) != v.RestoreSHA {
		return false, errClientConfigRecovery
	}
	return false, nil
}
func (n *nativeClientConfig) disposeNext(exact *configData, body []byte) error {
	if n.checkOrigin() != nil || exact == nil || exact.check() != nil {
		return errClientConfigRecovery
	}
	actual, e := exact.read(recoveryV2Budget)
	if e != nil || !bytes.Equal(actual, body) {
		return errClientConfigRecovery
	}
	return n.disposeOwnedFile(n.parent(), "recovery.next", exact)
}

type successorRecordIO struct{ *nativePrivateRecord }

func (r *successorRecordIO) closeWriter() error {
	if e := r.nativePrivateRecord.closeWriter(); e != nil {
		return r.n.successorFence()
	}
	return nil
}
func (n *nativeClientConfig) createSuccessor(body []byte) (*configData, error) {
	record := &successorRecordIO{&nativePrivateRecord{n: n, parent: n.parent(), name: "recovery.next"}}
	if durablePrivateRecord(body, record) != nil {
		return nil, errClientConfigRecovery
	}
	return record.held, nil
}

// The destination replacement is atomic within the cooperative fixed-name
// protocol under the SAME exclusive controller and held private parent. NT's
// rename-by-name is not a destination-ID CAS against arbitrary same-user edits.
// A failed/unknown call or close fences this process; never retry publication.
func (n *nativeClientConfig) publishSuccessor(old *configData, oldBody []byte, next *configData, nextBody []byte) (*configData, error) {
	if n.checkOrigin() != nil || old == nil || old.check() != nil || next == nil || next.check() != nil {
		return nil, errClientConfigRecovery
	}
	for _, r := range []struct {
		d    *configData
		body []byte
	}{{old, oldBody}, {next, nextBody}} {
		actual, e := r.d.read(recoveryV2Budget)
		if e != nil || !bytes.Equal(actual, r.body) {
			return nil, errClientConfigRecovery
		}
	}
	id, sd := next.id, next.sd
	recoveryQualificationCheckpoint(n, recoveryNextBeforeRename)
	if old.file.Close() != nil {
		return nil, n.successorFence()
	}
	old.file = nil
	name, _ := windows.UTF16FromString("recovery.intent")
	name = name[:len(name)-1]
	size := 20 + len(name)*2
	words := make([]uint64, (size+7)/8)
	buf := unsafe.Slice((*byte)(unsafe.Pointer(&words[0])), len(words)*8)
	buf[0] = 1
	*(*windows.Handle)(unsafe.Pointer(&buf[8])) = n.parent()
	*(*uint32)(unsafe.Pointer(&buf[16])) = uint32(len(name) * 2)
	for i, u := range name {
		*(*uint16)(unsafe.Pointer(&buf[20+i*2])) = u
	}
	var held *configData
	e := checkedPrivateMutation(&n.mutation, func() error {
		if n.checkOrigin() != nil || !n.mutationDataHeld(next) || next.check() != nil {
			return errClientConfigRecovery
		}
		actual, e := next.read(recoveryV2Budget)
		if e != nil || !bytes.Equal(actual, nextBody) {
			return errClientConfigRecovery
		}
		return nil
	}, func() error {
		var status windows.IO_STATUS_BLOCK
		e := windows.NtSetInformationFile(windows.Handle(next.file.Fd()), &status, &buf[0], uint32(size), 10)
		if e == nil && recoveryQualificationLost(n, recoveryLostMetadataRename) {
			return errClientConfigRecovery
		}
		return e
	}, func() error {
		if next.file.Close() != nil {
			return errClientConfigRecovery
		}
		next.file = nil
		h, e := configOpen(n.parent(), "recovery.intent", windows.FILE_OPEN, false, false, false, nil)
		if e != nil {
			return errClientConfigRecovery
		}
		held, e = n.hold(h, false)
		if e != nil || !sameConfigObject(id, held.id) || sd != held.sd {
			return errClientConfigRecovery
		}
		actual, e := held.read(recoveryV2Budget)
		if e != nil || !bytes.Equal(actual, nextBody) {
			return errClientConfigRecovery
		}
		return n.verifyMutationAbsence(n.parent(), "recovery.next")
	}, n.successorFence)
	if e != nil {
		return nil, e
	}
	recoveryQualificationCheckpoint(n, recoveryNextPublished)
	return held, nil
}

// NEXT is sidecar metadata only until every witness is checked and its complete
// target is durable. A prior full NEXT may name an older selector; explicit
// review can retire only that validated exact object while OLD remains intact.
func (n *nativeClientConfig) advanceRecovery(old *configData, oldBody []byte, next *configData, nextBody []byte, expected writerRecoveryV2, baseline []byte, profile string, recheck func() error) (*configData, []byte, error) {
	encoded, e := recoveryV2Bytes(expected)
	if e != nil {
		return nil, nil, e
	}

	var held *configData
	e = durableSuccessorTransition(func() error {
		if next != nil {
			partial, err := validateNextBody(nextBody, expected, baseline, profile)
			if err != nil {
				return err
			}
			if partial || !bytes.Equal(nextBody, encoded) {
				if n.disposeNext(next, nextBody) != nil {
					return errClientConfigRecovery
				}
				next = nil
			}
		}
		if next == nil {
			var err error
			next, err = n.createSuccessor(encoded)
			if err != nil {
				return err
			}
		}
		recoveryQualificationCheckpoint(n, recoveryNextDurable)
		return nil
	}, recheck, func() error { var err error; held, err = n.publishSuccessor(old, oldBody, next, encoded); return err })
	if e != nil {
		return nil, nil, e
	}
	return held, encoded, nil
}
