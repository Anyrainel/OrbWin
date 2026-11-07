//go:build linux

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestEditCrashChild(t *testing.T) {
	phase := os.Getenv("ORBWIN_FS_CRASH_PHASE")
	if phase == "" {
		t.Skip("subprocess fault injection")
	}
	_, err := edit(os.Getenv("ORBWIN_FS_TEST_ROOT"), editRequest{Path: "/config.txt", Expected: hash([]byte("original")), Text: "new content"}, func(current string) {
		if current == phase {
			os.Exit(91)
		}
	})
	t.Fatal("fault point not reached", err)
}
func TestEditProcessLossRetainsOriginalAndPreparedReceipt(t *testing.T) {
	for _, phase := range []string{"prepared", "exchanged"} {
		t.Run(phase, func(t *testing.T) {
			root := t.TempDir()
			file := filepath.Join(root, "config.txt")
			if err := os.WriteFile(file, []byte("original"), 0640); err != nil {
				t.Fatal(err)
			}
			child := exec.Command(os.Args[0], "-test.run=^TestEditCrashChild$")
			child.Env = append(os.Environ(), "ORBWIN_FS_CRASH_PHASE="+phase, "ORBWIN_FS_TEST_ROOT="+root)
			if err := child.Run(); err == nil {
				t.Fatal("process loss was not injected")
			}
			receipts, _ := filepath.Glob(filepath.Join(root, ".orbwin-edit-*.jsonl"))
			if len(receipts) != 1 {
				t.Fatal("prepared receipt was lost")
			}
			bytes, err := os.ReadFile(receipts[0])
			if err != nil {
				t.Fatal(err)
			}
			var receipt struct {
				Draft string `json:"draft"`
				Path  string `json:"path"`
			}
			if err = json.Unmarshal(bytes, &receipt); err != nil {
				t.Fatal(err)
			}
			current, _ := os.ReadFile(file)
			draft, _ := os.ReadFile(filepath.Join(root, receipt.Draft))
			if phase == "prepared" && (string(current) != "original" || string(draft) != "new content") {
				t.Fatal("pre-commit loss damaged versions")
			}
			if phase == "exchanged" && (string(current) != "new content" || string(draft) != "original") {
				t.Fatal("post-exchange loss discarded original")
			}
		})
	}
}
