package localnet

import (
	"encoding/json"
	"errors"
	"orbwin.local/session/internal/lanhelper"
)

type lanOptionsWire struct {
	Version   int    `json:"version"`
	Operation string `json:"operation"`
}
type lanReviewWire struct {
	Version   int          `json:"version"`
	Operation string       `json:"operation"`
	Revision  uint64       `json:"revision"`
	Selection LANSelection `json:"lanSelection"`
}
type lanConsumeWire struct {
	Version   int    `json:"version"`
	Operation string `json:"operation"`
	Revision  uint64 `json:"revision"`
	ReviewID  string `json:"lanReviewId"`
	ReviewSHA string `json:"lanReviewSha256"`
}
type lanOperationWire struct {
	Version     int    `json:"version"`
	Operation   string `json:"operation"`
	OperationID string `json:"lanOperationId"`
}
type lanReconcileWire struct {
	Version   int    `json:"version"`
	Operation string `json:"operation"`
	ReviewID  string `json:"lanReviewId"`
	ReviewSHA string `json:"lanReviewSha256"`
}
type lanLeaseWire struct {
	Version   int    `json:"version"`
	Operation string `json:"operation"`
	LeaseSHA  string `json:"lanLeaseSha256"`
}

func validateLANIPC(data []byte, operation string) error {
	var shape any
	var required int
	switch operation {
	case "lan-options":
		shape, required = lanOptionsWire{}, 2
	case "lan-reconcile-review", "lan-cleanup-review":
		shape, required = lanReconcileWire{}, 4
	case "lan-review":
		shape, required = lanReviewWire{}, 4
	case "lan-activate", "lan-remove", "lan-settle-remove", "lan-cleanup":
		shape, required = lanConsumeWire{}, 5
	case "lan-cancel", "lan-settle-helper":
		shape, required = lanOperationWire{}, 3
	case "lan-remove-review", "lan-settle-remove-review", "lan-inspect":
		shape, required = lanLeaseWire{}, 3
	default:
		return errors.New("unsupported fixed LAN operation")
	}
	if err := lanhelper.ValidateClosedJSON(data, shape); err != nil {
		return err
	}
	var members map[string]json.RawMessage
	if json.Unmarshal(data, &members) != nil || len(members) != required {
		return errors.New("complete exact LAN operation fields required")
	}
	return nil
}
