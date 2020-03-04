package stdchat

// CmdMsg is a msg for a command.
// The Message should be empty, it is reserved for future use.
type CmdMsg struct {
	BaseMsg
	Network EntityInfo `json:"net,omitempty"` // optional, if applicable.
	Command string     `json:"cmd"`
	Args    []string   `json:"args,omitempty"`
}

var _ Msger = &CmdMsg{}

func (msg CmdMsg) IsMsg() bool {
	return msg.BaseMsg.IsMsg() && msg.Command != ""
}

// GetNetwork gets the network, this is optional!
func (msg CmdMsg) GetNetwork() EntityInfo {
	return msg.Network
}

// NewCmd is a helper to create a command.
func NewCmd(id, command string, args ...string) *CmdMsg {
	msg := &CmdMsg{}
	msg.ID = id
	msg.Type = "cmd"
	msg.Command = command
	msg.Args = args
	return msg
}

// NewLogin is a login request.
func NewLogin(id, remote, userID, auth string) *CmdMsg {
	return NewCmd(id, "login", remote, userID, auth)
}

// NewLogout is a logout request.
// logoutID can be the network ID or conn ID (if applicable)
func NewLogout(id, logoutID string) *CmdMsg {
	return NewCmd(id, "logout", logoutID)
}

// NewLogoutReason is NewLogout with a reason for logging out.
// The reason may (or may not) be announced to chat users as a leave message.
func NewLogoutReason(id, logoutID, reason string) *CmdMsg {
	return NewCmd(id, "logout", logoutID, reason)
}

// NewRaw is a helper to create a raw command.
func NewRaw(id, netID string, args ...string) *CmdMsg {
	cmd := NewCmd(id, "raw", args...)
	cmd.Network.Init(netID, "net")
	return cmd
}
