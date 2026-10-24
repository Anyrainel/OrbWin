// OrbWin credential protocol adapter. Docker owns registry authentication;
// Windows owns secret persistence. Never log arguments, input or stored values.
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/docker/docker-credential-helpers/credentials"
)

const prefix = "OrbWin/registry/v1/"
const owner = "OrbWin Registry Credentials v1"
const inputLimit = 16384

var missing = errors.New("credential not found")

type entry struct{ key, username, secret, label string }
type vault interface {
	get(string) (entry, error)
	put(entry) error
	remove(string) error
	list(string) ([]entry, error)
}
type helper struct{ vault vault }

func keyFor(server string) (string, error) {
	if server == "" || len(server) > 2048 || !utf8.ValidString(server) || strings.TrimSpace(server) != server || strings.ContainsFunc(server, unicode.IsControl) {
		return "", errors.New("Registry name is empty, invalid or exceeds 2048 bytes")
	}
	return prefix + base64.RawURLEncoding.EncodeToString([]byte(server)), nil
}
func (h helper) Add(c *credentials.Credentials) error {
	key, err := keyFor(c.ServerURL)
	if err != nil {
		return err
	}
	if c.Username == "" || len(c.Username) > 512 || !utf8.ValidString(c.Username) || strings.ContainsRune(c.Username, 0) || len(c.Secret) > 2560 || !utf8.ValidString(c.Secret) {
		return errors.New("Username is invalid or exceeds 512 bytes, or the credential exceeds the Windows 2560-byte limit")
	}
	old, err := h.vault.get(key)
	if err != nil && !errors.Is(err, missing) {
		return errors.New("Cannot read the Windows credential store")
	}
	if err == nil && old.label != owner {
		return errors.New("The credential target has another owner; it was left unchanged")
	}
	if err = h.vault.put(entry{key: key, username: c.Username, secret: c.Secret, label: owner}); err != nil {
		return errors.New("Windows could not save the credential")
	}
	return nil
}
func (h helper) Get(server string) (string, string, error) {
	key, err := keyFor(server)
	if err != nil {
		return "", "", err
	}
	value, err := h.vault.get(key)
	if errors.Is(err, missing) {
		return "", "", credentials.NewErrCredentialsNotFound()
	}
	if err != nil {
		return "", "", errors.New("Cannot read the Windows credential store")
	}
	if value.label != owner {
		return "", "", errors.New("The credential target has another owner")
	}
	return value.username, value.secret, nil
}
func (h helper) Delete(server string) error {
	key, err := keyFor(server)
	if err != nil {
		return err
	}
	value, err := h.vault.get(key)
	if errors.Is(err, missing) {
		return nil
	}
	if err != nil {
		return errors.New("Cannot read the Windows credential store")
	}
	if value.label != owner {
		return errors.New("The credential target has another owner; it was left unchanged")
	}
	if err = h.vault.remove(key); err != nil {
		return errors.New("Windows could not remove the credential")
	}
	return nil
}
func (h helper) List() (map[string]string, error) {
	entries, err := h.vault.list(prefix + "*")
	if err != nil {
		return nil, errors.New("Cannot list OrbWin registry credentials")
	}
	result := map[string]string{}
	for _, item := range entries {
		if item.label != owner || !strings.HasPrefix(item.key, prefix) {
			continue
		}
		server, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(item.key, prefix))
		if err != nil {
			return nil, errors.New("An OrbWin credential target is malformed")
		}
		key, err := keyFor(string(server))
		if err != nil || key != item.key {
			return nil, errors.New("An OrbWin credential target is malformed")
		}
		result[string(server)] = item.username
		if len(result) > 256 {
			return nil, errors.New("OrbWin registry list exceeds 256 entries")
		}
	}
	return result, nil
}

func run(args []string, in io.Reader, out io.Writer, store vault) error {
	if len(args) != 1 {
		return errors.New("Usage: docker-credential-orbwin <store|get|erase|list>")
	}
	action := args[0]
	if action != "store" && action != "get" && action != "erase" && action != "list" {
		return errors.New("Unsupported OrbWin credential operation")
	}
	var input []byte
	if action != "list" {
		var err error
		input, err = io.ReadAll(io.LimitReader(in, inputLimit+1))
		if err != nil || len(input) > inputLimit {
			return errors.New("Credential input could not be read or exceeds 16 KiB")
		}
		if action == "store" && !json.Valid(input) {
			return errors.New("Credential input is not valid JSON")
		}
	}
	// Buffer before publication so a rejected operation cannot emit partial data.
	var output bytes.Buffer
	if err := credentials.HandleCommand(helper{vault: store}, action, bytes.NewReader(input), &output); err != nil {
		if credentials.IsErrCredentialsNotFound(err) {
			return credentials.NewErrCredentialsNotFound()
		}
		// Parser errors may quote input. Only our fixed adapter errors are safe.
		if strings.HasPrefix(err.Error(), "invalid character") || strings.HasPrefix(err.Error(), "json:") || errors.Is(err, io.EOF) {
			return errors.New("Credential input has an invalid shape")
		}
		return err
	}
	if output.Len() > 1<<20 {
		return errors.New("Credential output exceeds 1 MiB")
	}
	_, err := io.Copy(out, &output)
	return err
}

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, windowsVault{}); err != nil {
		fmt.Fprintln(os.Stdout, err)
		os.Exit(1)
	}
}
