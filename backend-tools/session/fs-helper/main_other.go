//go:build !linux

package main

import (
	"fmt"
	"os"
)

func main() { fmt.Fprintln(os.Stderr, "OrbWin guest file transactions require Linux"); os.Exit(125) }
