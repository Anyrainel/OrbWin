package main

import (
	"strings"
	"testing"
)

func TestCacheRoleRequiresIndependentCompletePins(t *testing.T) {
	old := []string{buildLayout, patchClosureSHA, rddSHA, forwarderSHA, credentialsSHA, dockerSHA, composeSHA, buildxSHA, toolLockSHA}
	defer func() {
		buildLayout, patchClosureSHA, rddSHA, forwarderSHA, credentialsSHA, dockerSHA, composeSHA, buildxSHA, toolLockSHA = old[0], old[1], old[2], old[3], old[4], old[5], old[6], old[7], old[8]
	}()
	buildLayout = originLayout
	patchClosureSHA = strings.Repeat("a", 64)
	rddSHA = strings.Repeat("b", 64)
	forwarderSHA = ""
	if _, e := cacheRole("rdd"); e == nil {
		t.Fatal("RDD admitted without independently pinned forwarder")
	}
	forwarderSHA = strings.Repeat("c", 64)
	s, e := cacheRole("rdd")
	if e != nil || s.directory != "rdd-"+rddSHA || len(s.files) != 2 {
		t.Fatal(s, e)
	}
	dockerSHA = dockerArgumentSchemaSHA
	toolLockSHA = strings.Repeat("d", 64)
	composeSHA = strings.Repeat("e", 64)
	buildxSHA = strings.Repeat("f", 64)
	s, e = cacheRole("docker")
	if e != nil || s.directory != "docker-"+toolLockSHA || len(s.files) != 3 {
		t.Fatal(s, e)
	}
	dockerSHA = strings.Repeat("1", 64)
	if _, e = cacheRole("docker"); e == nil {
		t.Fatal("argument-schema mismatch admitted")
	}
	if _, e = cacheRole("caller-path"); e == nil {
		t.Fatal("unknown role admitted")
	}
	buildLayout = "legacy"
	if _, e = cacheRole("rdd"); e == nil {
		t.Fatal("legacy layout admitted")
	}
}
func TestCachePinShapeRejectsZeroAndNonCanonical(t *testing.T) {
	for _, s := range []string{"", strings.Repeat("0", 64), strings.Repeat("A", 64), strings.Repeat("a", 63)} {
		if canonicalPin(s) {
			t.Fatal(s)
		}
	}
}
func TestDetachedAdmissionIsOnlyClosedServiceStart(t *testing.T) {
	good := []string{"service", "start", "--controllers=*,-path-management", "--timeout=90s"}
	if !exactDetachedStart(good) {
		t.Fatal("closed launch lost")
	}
	for _, bad := range [][]string{{"service", "start"}, {"service", "start", "--controllers=*,-path-management", "--timeout=90s", "--detach"}, {"service", "restart", "--controllers=*,-path-management", "--timeout=90s"}, {"docker", "exec"}} {
		if exactDetachedStart(bad) {
			t.Fatal(bad)
		}
	}
}
func TestChildPATHUsesRetainedCacheOnly(t *testing.T) {
	t.Setenv("PATH", "user-tools")
	env := cleanEnv(target{runtime: "UNTRUSTED-PUBLIC-RESOURCES", credentialDirectory: "held-credential-cache", dockerDirectory: "held-docker-cache"}, false)
	for _, entry := range env {
		if strings.HasPrefix(strings.ToUpper(entry), "PATH=") {
			if strings.Contains(entry, "UNTRUSTED") || !strings.Contains(entry, "held-credential-cache") || !strings.Contains(entry, "held-docker-cache") || !strings.HasSuffix(entry, "user-tools") {
				t.Fatal(entry)
			}
			return
		}
	}
	t.Fatal("PATH missing")
}
