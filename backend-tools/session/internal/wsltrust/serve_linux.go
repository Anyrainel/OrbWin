//go:build linux

package wsltrust

import (
	"context"
	"encoding/json"
	"errors"
	"io"
)

func ServeSystem(ctx context.Context, input io.Reader, output io.Writer) error {
	data, err := io.ReadAll(io.LimitReader(input, RequestBudget+1))
	if err != nil {
		return err
	}
	if len(data) == 0 || len(data) > RequestBudget {
		return errors.New("selected distro trust request exceeds its budget")
	}
	var request Request
	if err := StrictJSON(data, &request); err != nil {
		return err
	}
	if _, err := request.Validate(); err != nil {
		return err
	}
	store, err := NewSystemStore()
	if err != nil {
		return err
	}
	status, applyErr := store.Apply(ctx, request)
	applyErr = errors.Join(applyErr, store.Close())
	response := Response{Version: 1, Status: &status}
	if applyErr != nil {
		response.Error = applyErr.Error()
		if len(response.Error) > 4096 {
			response.Error = response.Error[:4096]
		}
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		return err
	}
	if len(encoded) > ResponseBudget {
		return errors.New("selected distro trust response exceeds its budget")
	}
	_, err = output.Write(append(encoded, '\n'))
	return err
}
