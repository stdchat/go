package stdchat

// Statuser has status info.
type Statuser interface {
	GetType() string
	GetProtocol() string
	String() string
}

// ProtocolStateInfo is protocol state information.
type ProtocolStateInfo struct {
	TypeInfo            // proto-state
	Protocol string     `json:"proto"`
	Values   ValuesInfo `json:"values,omitempty"`
}

func (x ProtocolStateInfo) GetProtocol() string {
	return x.Protocol
}

func (x ProtocolStateInfo) String() string {
	return "Protocol: " + x.Protocol
}

// NetworkStateInfo is network state information.
type NetworkStateInfo struct {
	TypeInfo              // network-state
	Network    EntityInfo `json:"net"`
	Protocol   string     `json:"proto"`
	Connection EntityInfo `json:"conn,omitempty"` // Optional, if connections.
	Myself     EntityInfo `json:"myself"`
	Values     ValuesInfo `json:"values,omitempty"`
	Ready      bool       `json:"ready"`
}

func (x NetworkStateInfo) GetProtocol() string {
	return x.Protocol
}

func (x NetworkStateInfo) String() string {
	return "Network: " + x.Network.GetDisplayName() +
		" | Protocol: " + x.Protocol
}

// SubscriptionStateInfo is subscription state information.
// HistoryURL can be a URL with a known JSON REST API to fetch history, if supported.
// TODO: define history API.
type SubscriptionStateInfo struct {
	TypeInfo                 // subscription-state
	Network     EntityInfo   `json:"net"`
	Protocol    string       `json:"proto"`
	Destination EntityInfo   `json:"dest"`
	Subject     MessageInfo  `json:"subject,omitempty"`
	Members     []MemberInfo `json:"members,omitempty"`
	Values      ValuesInfo   `json:"values,omitempty"`
	HistoryURL  string       `json:"history,omitempty"` // empty if not supported.
}

func (x SubscriptionStateInfo) GetProtocol() string {
	return x.Protocol
}

func (x SubscriptionStateInfo) String() string {
	return "Subscription: " + x.Destination.GetDisplayName() +
		" | Network: " + x.Network.GetDisplayName() +
		" | Protocol: " + x.Protocol
}
