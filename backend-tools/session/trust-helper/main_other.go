//go:build !linux

package main

import (
	"fmt"
	"os"
)

func main() { fmt.Fprintln(os.Stderr, "selected distro trust helper requires Linux"); os.Exit(125) }
