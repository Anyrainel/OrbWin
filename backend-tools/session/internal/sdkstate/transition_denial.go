package sdkstate

import (
	"bytes"
	"encoding/json"
)

type transitionDenialObject struct {
	Volume uint32 `json:"volume"`
	High   uint32 `json:"high"`
	Low    uint32 `json:"low"`
}
type transitionDenialRecord struct {
	Schema                                                               int
	Transaction                                                          string
	Root, Controller                                                     transitionDenialObject
	Before, After                                                        uint32
	RootBeforeACL, RootAfterACL, ControllerBeforeACL, ControllerAfterACL [32]byte
	Stage                                                                string
}

func parseTransitionDenial(body []byte, scope TransitionScope, root, controller transitionDenialObject, attrs uint32) (transitionDenialRecord, error) {
	var record transitionDenialRecord
	if len(body) == 0 || len(body) > 4096 || json.Unmarshal(body, &record) != nil {
		return record, ErrRecoveryRequired
	}
	canonical, _ := json.Marshal(record)
	// FILE_ATTRIBUTE_READONLY=1, ARCHIVE=0x20, NORMAL=0x80 (Windows SDK).
	if !bytes.Equal(canonical, body) || record.Schema != 1 || record.Transaction != scope.Transaction || !transitionID(scope.Transaction) || record.Stage != "readonly-owner-acquired" || record.Before & ^uint32(0x80|0x20) != 0 || record.After != (record.Before & ^uint32(0x80))|1 || attrs != record.After || record.Root != root || record.Controller != controller || root.High|root.Low == 0 || controller.High|controller.Low == 0 || root == controller || record.RootBeforeACL == [32]byte{} || record.RootAfterACL == [32]byte{} || record.ControllerBeforeACL == [32]byte{} || record.ControllerAfterACL == [32]byte{} {
		return transitionDenialRecord{}, ErrRecoveryRequired
	}
	return record, nil
}
