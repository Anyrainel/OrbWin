package main

import (
	"errors"
	"golang.org/x/sys/windows/registry"
	"path/filepath"
	"strings"
)

func verifyRegistration(home string) error {
	root, e := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Lxss`, registry.READ)
	if e != nil {
		return e
	}
	defer root.Close()
	names, e := root.ReadSubKeyNames(-1)
	if e != nil {
		return e
	}
	count := 0
	for _, name := range names {
		k, e := registry.OpenKey(root, name, registry.READ)
		if e != nil {
			return e
		}
		distribution, _, e := k.GetStringValue("DistributionName")
		if e != nil {
			k.Close()
			return e
		}
		if strings.EqualFold(distribution, "lima-orbwin") {
			count++
			base, _, e := k.GetStringValue("BasePath")
			k.Close()
			if e != nil {
				return e
			}
			base, e = filepath.EvalSymlinks(base)
			if e != nil {
				return e
			}
			relative, e := filepath.Rel(home, base)
			if e != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				return errors.New("owned WSL registration is outside the recorded home")
			}
		} else {
			k.Close()
		}
	}
	if count != 1 {
		return errors.New("exactly one owned WSL registration is required")
	}
	return nil
}
