// Test-fixture password hashing only. Real product credentials use the Windows
// store, never a file. Receive the disposable fixture password through stdin.
package main

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	password, err := io.ReadAll(io.LimitReader(os.Stdin, 73))
	if err != nil || len(password) == 0 || len(password) > 72 {
		fmt.Fprintln(os.Stderr, "Invalid disposable fixture input")
		os.Exit(1)
	}
	hash, err := bcrypt.GenerateFromPassword(password, 10)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Cannot hash the disposable fixture credential")
		os.Exit(1)
	}
	fmt.Print(string(hash))
}
