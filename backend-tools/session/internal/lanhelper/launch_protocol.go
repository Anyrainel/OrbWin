package lanhelper

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"strconv"
)

const launchFrameBudget = 4096

type launchFrame struct {
	Version      int    `json:"version"`
	Kind         string `json:"kind"`
	SourceHandle string `json:"sourceHandle,omitempty"`
	PID          uint32 `json:"pid,omitempty"`
	Creation     string `json:"creation,omitempty"`
	ErrorCode    string `json:"errorCode,omitempty"`
}

func launchPipeName(installation, transaction string) (string, error) {
	if !recordID(installation) || !recordID(transaction) {
		return "", errors.New("fixed launcher channel identities required")
	}
	return `\\.\pipe\OrbWinLANLaunch-v1-` + installation + "-" + transaction, nil
}
func (frame launchFrame) validate() error {
	if frame.Version != 1 {
		return errors.New("launcher version differs")
	}
	switch frame.Kind {
	case "handle":
		handle, err := strconv.ParseUint(frame.SourceHandle, 10, 64)
		creation, e := strconv.ParseUint(frame.Creation, 10, 64)
		if err != nil || e != nil || handle == 0 || creation == 0 || strconv.FormatUint(handle, 10) != frame.SourceHandle || strconv.FormatUint(creation, 10) != frame.Creation || frame.PID == 0 || frame.ErrorCode != "" {
			return errors.New("launcher native handle metadata differs")
		}
	case "received", "helper-ready":
		if frame.SourceHandle != "" || frame.PID != 0 || frame.Creation != "" || frame.ErrorCode != "" {
			return errors.New("launcher receipt shape differs")
		}
	case "error":
		if frame.SourceHandle != "" || frame.PID != 0 || frame.Creation != "" || (frame.ErrorCode != "declined" && frame.ErrorCode != "unknown") {
			return errors.New("launcher error shape differs")
		}
	default:
		return errors.New("unsupported fixed launcher message")
	}
	return nil
}
func readLaunch(reader io.Reader) (launchFrame, error) {
	var empty launchFrame
	var header [4]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return empty, err
	}
	size := binary.LittleEndian.Uint32(header[:])
	if size == 0 || size > launchFrameBudget {
		return empty, errors.New("launcher frame budget exceeded")
	}
	data := make([]byte, size)
	if _, err := io.ReadFull(reader, data); err != nil {
		return empty, err
	}
	var frame launchFrame
	if ValidateClosedJSON(data, launchFrame{}) != nil || json.Unmarshal(data, &frame) != nil || frame.validate() != nil {
		return empty, errors.New("launcher closed frame differs")
	}
	var members map[string]json.RawMessage
	_ = json.Unmarshal(data, &members)
	required := 2
	if frame.Kind == "handle" {
		required = 5
	} else if frame.Kind == "error" {
		required = 3
	}
	if len(members) != required {
		return empty, errors.New("launcher tag has unrelated members")
	}
	return frame, nil
}
func writeLaunch(writer io.Writer, frame launchFrame) error {
	if err := frame.validate(); err != nil {
		return err
	}
	data, err := json.Marshal(frame)
	if err != nil || len(data) > launchFrameBudget {
		return errors.New("launcher encoded frame too large")
	}
	var header [4]byte
	binary.LittleEndian.PutUint32(header[:], uint32(len(data)))
	for _, part := range [][]byte{header[:], data} {
		for len(part) > 0 {
			n, err := writer.Write(part)
			if err != nil {
				return err
			}
			if n <= 0 || n > len(part) {
				return io.ErrShortWrite
			}
			part = part[n:]
		}
	}
	return nil
}
