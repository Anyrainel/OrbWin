package localnet

// LAN options are bounded authoritative observations for user selection. They
// are NOT activation authority; Review independently re-observes every choice.
// No caller identity, native receipt/rule key or executable path is exposed.
type LANPublicationChoice struct {
	ContainerID        string `json:"containerId"`
	ContainerName      string `json:"containerName"`
	StartedAt          string `json:"startedAt"`
	HostPort           uint16 `json:"hostPort"`
	ContainerPort      uint16 `json:"containerPort"`
	Protocol           string `json:"protocol"`
	Available          bool   `json:"available"`
	AvailabilityReason string `json:"availabilityReason,omitempty"`
}
type LANInterfaceChoice struct {
	InterfaceGUID      string   `json:"interfaceGuid"`
	InterfaceLUID      string   `json:"interfaceLuid"`
	Alias              string   `json:"alias"`
	PrivateAddresses   []string `json:"privateAddresses"`
	Profile            string   `json:"profile"` // private/domain/public/unknown
	Available          bool     `json:"available"`
	AvailabilityReason string   `json:"availabilityReason,omitempty"`
}
type LANOptions struct {
	Version                 int                    `json:"version"`
	ReadAt                  string                 `json:"readAt"`
	ExpiresAt               string                 `json:"expiresAt"`
	Complete                bool                   `json:"complete"`
	SourceAvailable         bool                   `json:"sourceAvailable"`
	SourceUnavailableReason string                 `json:"sourceUnavailableReason,omitempty"`
	HelperAvailable         bool                   `json:"helperAvailable"`
	HelperUnavailableReason string                 `json:"helperUnavailableReason,omitempty"`
	Publications            []LANPublicationChoice `json:"publications"`
	Interfaces              []LANInterfaceChoice   `json:"interfaces"`
}

type LANSelection struct {
	ContainerID    string   `json:"containerId"`
	StartedAt      string   `json:"startedAt"`
	HostPort       uint16   `json:"hostPort"`
	ContainerPort  uint16   `json:"containerPort"`
	Protocol       string   `json:"protocol"`
	InterfaceGUID  string   `json:"interfaceGuid"`
	InterfaceLUID  string   `json:"interfaceLuid"`
	LocalAddress   string   `json:"localAddress"`
	Profile        string   `json:"profile"`
	ClientNetworks []string `json:"clientNetworks"`
}
type LANReview struct {
	ReviewID              string       `json:"reviewId"`
	ReviewSHA256          string       `json:"reviewSha256"`
	ExpiresAt             string       `json:"expiresAt"`
	Selection             LANSelection `json:"selection"`
	PublicationGeneration uint64       `json:"publicationGeneration,string"`
	Purpose               string       `json:"purpose"`
	Available             bool         `json:"available"`
	ReasonCode            string       `json:"reasonCode,omitempty"`
}
