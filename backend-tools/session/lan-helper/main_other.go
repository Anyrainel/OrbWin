//go:build !windows || !amd64

package main

import "os"

func main() { os.Exit(125) }
