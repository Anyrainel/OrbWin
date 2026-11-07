package main

import (
	"errors"
	"strings"
)

// Pins are compiled from independently verified build inputs. runtime.json is
// only a selected-target compatibility record and cannot authorize executable bytes.
var rddSHA, forwarderSHA, credentialsSHA, composeSHA, buildxSHA, toolLockSHA, buildLayout, patchClosureSHA string

const originLayout = "native-profile-runtime-v1"

type cacheFile struct {
	name, subdir, sha string
	limit             uint64
}
type cacheSpec struct {
	directory string
	files     []cacheFile
}

func canonicalPin(s string) bool {
	if len(s) != 64 || s == strings.Repeat("0", 64) {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func cacheRole(role string) (cacheSpec, error) {
	refusal := errors.New("CLI build lacks qualified private-cache pins; prepare a matching OrbWin desktop build")
	if buildLayout != originLayout || !canonicalPin(patchClosureSHA) {
		return cacheSpec{}, refusal
	}
	var s cacheSpec
	switch role {
	case "rdd":
		s = cacheSpec{"rdd-" + rddSHA, []cacheFile{{"rdd.exe", "", rddSHA, 512 << 20}, {"rdd-forwarder.exe", "", forwarderSHA, 4 << 20}}}
	case "docker":
		if !canonicalPin(toolLockSHA) || dockerSHA != dockerArgumentSchemaSHA {
			return cacheSpec{}, refusal
		}
		s = cacheSpec{"docker-" + toolLockSHA, []cacheFile{{"docker.exe", "", dockerSHA, 128 << 20}, {"docker-compose.exe", "docker-cli-plugins", composeSHA, 256 << 20}, {"docker-buildx.exe", "docker-cli-plugins", buildxSHA, 256 << 20}}}
	case "credentials":
		s = cacheSpec{"docker-credential-orbwin-" + credentialsSHA, []cacheFile{{"docker-credential-orbwin.exe", "", credentialsSHA, 16 << 20}}}
	default:
		return cacheSpec{}, refusal
	}
	for _, f := range s.files {
		if !canonicalPin(f.sha) {
			return cacheSpec{}, refusal
		}
	}
	return s, nil
}

// Only the fixed desiredState launcher may outlive the CLI. Arbitrary rdd
// commands/plugins cannot request breakaway or detached treatment.
func exactDetachedStart(args []string) bool {
	return len(args) == 4 && args[0] == "service" && args[1] == "start" && args[2] == "--controllers=*,-path-management" && args[3] == "--timeout=90s"
}
