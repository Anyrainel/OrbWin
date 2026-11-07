//go:build !windows

package localnet

import (
	"context"
	"errors"
)

func ServeServiceIPC(context.Context, *ServiceRuntime, string, string) error {
	return errors.New("Windows user-scoped local-services IPC required")
}
func CallServiceIPC(context.Context, string, string, ServiceRequest) (ServiceResponse, error) {
	return ServiceResponse{}, errors.New("Windows user-scoped local-services IPC required")
}
