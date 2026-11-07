package localnet

// This module is a side-effect-free review/guard boundary. It cannot bind a
// listener, launch a privileged helper or authorize a firewall mutation. The
// eventual native owner must obtain observations itself, not trust renderer data.
import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/netip"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

type LANPublication struct {
	Daemon        string `json:"daemon"`
	ContainerID   string `json:"containerId"`
	StartedAt     string `json:"startedAt"`
	Generation    uint64 `json:"generation"`
	HostPort      uint16 `json:"hostPort"`
	ContainerPort uint16 `json:"containerPort"`
	Protocol      string `json:"protocol"`
}

type LANExposureReview struct {
	Version        int            `json:"version"`
	Owner          string         `json:"owner"`
	UserSID        string         `json:"userSid"`
	ProgramSHA256  string         `json:"programSha256"`
	Publication    LANPublication `json:"publication"`
	InterfaceLUID  string         `json:"interfaceLuid"`
	LocalAddress   string         `json:"localAddress"`
	Profile        string         `json:"profile"`
	ClientNetworks []string       `json:"clientNetworks"`
}

type LANExposureObservation struct {
	Publication                          LANPublication
	PublicationActive                    bool
	InterfaceLUID, LocalAddress, Profile string
	InterfacePresent                     bool
	ReadAt                               time.Time
}

type LANExposurePlan struct {
	Review   LANExposureReview `json:"review"`
	SHA256   string            `json:"sha256"`
	RuleName string            `json:"ruleName"`
	// Activation remains unavailable until the dedicated native transaction and
	// its ownership receipts exist. A plan is never evidence of exposure/consent.
	ActivationAvailable bool `json:"activationAvailable"`
}

type LANFirewallRule struct {
	Name, Owner, ProgramSHA256, LocalAddress, Protocol, Profile string
	LocalPort                                                   uint16
	ClientNetworks                                              []string
}

var lanSID = regexp.MustCompile(`^S-1-5-21-[0-9]{1,10}-[0-9]{1,10}-[0-9]{1,10}-[0-9]{1,10}$`)

func (review LANExposureReview) validate() error {
	owner, err := hex.DecodeString(review.Owner)
	if err != nil || len(owner) != 16 || hex.EncodeToString(owner) != review.Owner || review.Version != 1 || !lanSID.MatchString(review.UserSID) || !fullID(review.ProgramSHA256) {
		return errors.New("LAN review requires version, installation owner, ordinary user SID and attested program digest")
	}
	for _, part := range strings.Split(strings.TrimPrefix(review.UserSID, "S-1-5-21-"), "-") {
		value, err := strconv.ParseUint(part, 10, 32)
		if err != nil || strconv.FormatUint(value, 10) != part {
			return errors.New("LAN review requires a canonical ordinary user SID")
		}
	}
	publication := review.Publication
	started, timeErr := time.Parse(time.RFC3339Nano, publication.StartedAt)
	if !selectorText(publication.Daemon) || !fullID(publication.ContainerID) || timeErr != nil || started.IsZero() || publication.Generation == 0 || publication.HostPort == 0 || publication.ContainerPort == 0 || (publication.Protocol != "tcp" && publication.Protocol != "udp") {
		return errors.New("LAN review requires an exact running publication and incarnation/generation")
	}
	luid, err := strconv.ParseUint(review.InterfaceLUID, 10, 64)
	if err != nil || luid == 0 || strconv.FormatUint(luid, 10) != review.InterfaceLUID {
		return errors.New("LAN review requires an exact canonical interface LUID")
	}
	address, err := netip.ParseAddr(review.LocalAddress)
	if err != nil || !address.Is4() || !address.IsPrivate() || address.String() != review.LocalAddress {
		return errors.New("LAN exposure requires an explicitly selected private IPv4 interface address")
	}
	if review.Profile != "private" && review.Profile != "domain" {
		return errors.New("LAN exposure requires explicitly reviewed Private or Domain profile; Public is refused")
	}
	if len(review.ClientNetworks) == 0 || len(review.ClientNetworks) > 8 {
		return errors.New("review one to eight explicit private client IPv4 networks")
	}
	last := ""
	for _, raw := range review.ClientNetworks {
		prefix, err := netip.ParsePrefix(raw)
		if err != nil || !prefix.Addr().Is4() || !prefix.Addr().IsPrivate() || prefix.Masked() != prefix || prefix.String() != raw || raw <= last {
			return errors.New("client networks must be sorted distinct canonical private IPv4 CIDRs")
		}
		// The prefix must fit entirely inside one RFC1918 block, not merely start
		// there. This rejects 10.0.0.0/1 and private-starting broad allow rules.
		contained := false
		for _, block := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"} {
			parent := netip.MustParsePrefix(block)
			if parent.Contains(prefix.Addr()) && prefix.Bits() >= parent.Bits() {
				contained = true
			}
		}
		if !contained {
			return errors.New("client network extends outside a private IPv4 range")
		}
		last = raw
	}
	return nil
}

func PlanLANExposure(review LANExposureReview) (LANExposurePlan, error) {
	if err := review.validate(); err != nil {
		return LANExposurePlan{}, err
	}
	// Freeze slice ownership before returning a review for consent. Neither the
	// caller's draft nor a later renderer edit can alter the hashed plan.
	review.ClientNetworks = append([]string{}, review.ClientNetworks...)
	data, err := json.Marshal(review)
	if err != nil {
		return LANExposurePlan{}, err
	}
	digest := sha256.Sum256(append([]byte("orbwin-lan-review-v1\n"), data...))
	hash := hex.EncodeToString(digest[:])
	return LANExposurePlan{Review: review, SHA256: hash, RuleName: "OrbWin-LAN-" + review.Owner + "-" + hash[:16]}, nil
}

// Recheck immediately before each future journal/native/listener boundary. A
// refreshed observation cannot silently amend the user's reviewed endpoint.
func GuardLANExposure(plan LANExposurePlan, observed LANExposureObservation, now time.Time) error {
	expected, err := PlanLANExposure(plan.Review)
	if err != nil || plan.ActivationAvailable || expected.SHA256 != plan.SHA256 || expected.RuleName != plan.RuleName {
		return errors.New("LAN review plan changed or incorrectly claims activation support")
	}
	if observed.ReadAt.IsZero() || now.Before(observed.ReadAt) || now.Sub(observed.ReadAt) > 5*time.Second || !observed.PublicationActive || !observed.InterfacePresent || observed.Publication != plan.Review.Publication || observed.InterfaceLUID != plan.Review.InterfaceLUID || observed.LocalAddress != plan.Review.LocalAddress || observed.Profile != plan.Review.Profile {
		return errors.New("LAN publication/interface observation is stale, unavailable or differs from consent; review again")
	}
	return nil
}

// Packet admission must enforce the same client scope in the unprivileged
// listener as in Windows Firewall, including when another broad allow rule is
// present. This pure predicate grants no listener or firewall authority.
func (plan LANExposurePlan) AllowsPeer(peer netip.Addr) bool {
	expected, err := PlanLANExposure(plan.Review)
	if !peer.IsValid() || !peer.Is4() || err != nil || expected.SHA256 != plan.SHA256 || expected.RuleName != plan.RuleName || plan.ActivationAvailable {
		return false
	}
	for _, raw := range plan.Review.ClientNetworks {
		prefix, err := netip.ParsePrefix(raw)
		if err == nil && prefix.Contains(peer) {
			return true
		}
	}
	return false
}

func ValidateLANRemoval(plan LANExposurePlan, observed LANFirewallRule) error {
	expected, err := PlanLANExposure(plan.Review)
	if err != nil || plan.ActivationAvailable || expected.SHA256 != plan.SHA256 || expected.RuleName != plan.RuleName || observed.Name != plan.RuleName || observed.Owner != plan.Review.Owner || observed.ProgramSHA256 != plan.Review.ProgramSHA256 || observed.LocalAddress != plan.Review.LocalAddress || observed.Protocol != plan.Review.Publication.Protocol || observed.LocalPort != plan.Review.Publication.HostPort || observed.Profile != plan.Review.Profile || !slices.Equal(observed.ClientNetworks, plan.Review.ClientNetworks) {
		return errors.New("LAN removal requires the exact owned rule and retained review; preserve foreign or altered rules")
	}
	return nil
}
