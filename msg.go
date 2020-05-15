package stdchat

import (
	"time"
)

// Msger is anything with a common msg type.
type Msger interface {
	IsMsg() bool
	GetProtocol() string
	GetType() string
}

// BaseMsg is the basis for a msg.
type BaseMsg struct {
	TypeInfo             // msg/* info/* group-enter/* other/* ...
	ID       string      `json:"id"`
	Protocol string      `json:"proto,omitempty"` // empty if not from the protocol.
	Time     time.Time   `json:"time,omitempty"`  // can be zero if from client.
	Message  MessageInfo `json:"msg,omitempty"`
	Values   ValuesInfo  `json:"values,omitempty"`
}

var _ BaseMsger = &BaseMsg{}

func (msg *BaseMsg) Init(id, typ, protocol string) {
	msg.ID = id
	msg.Type = typ
	msg.Protocol = protocol
	msg.Time = time.Now()
}

func (msg *BaseMsg) IsMsg() bool {
	return msg.Type != ""
}

func (msg *BaseMsg) GetProtocol() string {
	return msg.Protocol
}

func (msg *BaseMsg) GetBaseMsg() *BaseMsg {
	return msg
}

func (msg BaseMsg) GetID() string {
	return msg.ID
}

func (msg BaseMsg) GetMessage(msgType string) Message {
	return msg.Message.Get(msgType)
}

func (msg BaseMsg) GetMessageString() string {
	return msg.Message.String()
}

// BaseMsger is anything based on BaseMsg.
type BaseMsger interface {
	Msger
	GetBaseMsg() *BaseMsg
	GetID() string
	GetMessage(msgType string) Message
	GetMessageString() string
}

// NetMsg is a generic network message.
type NetMsg struct {
	BaseMsg
	Network EntityInfo `json:"net"`
}

func (msg *NetMsg) Init(id, typ, protocol, networkID string) {
	msg.BaseMsg.Init(id, typ, protocol)
	msg.Network.Init(networkID, "net")
}

func (msg *NetMsg) IsMsg() bool {
	return msg.BaseMsg.IsMsg() && msg.Network.ID != ""
}

func (msg NetMsg) GetNetwork() EntityInfo {
	return msg.Network
}

// NetMsger is any base msg which also has a network.
type NetMsger interface {
	BaseMsger
	GetNetwork() EntityInfo
}

// ChatMsg is a chat msg.
type ChatMsg struct {
	NetMsg
	Destination EntityInfo  `json:"dest,omitempty"`
	From        EntityInfo  `json:"from,omitempty"`
	ReplyToID   string      `json:"replyTo,omitempty"`
	Attachments []MediaInfo `json:"attachments,omitempty"`
}

var _ ChatMsger = &ChatMsg{}

func (msg *ChatMsg) Init(id, typ, protocol, networkID string) {
	msg.NetMsg.Init(id, typ, protocol, networkID)
}

func (msg *ChatMsg) IsMsg() bool {
	return msg.BaseMsg.IsMsg() && msg.Network.ID != "" &&
		(msg.Destination.ID != "" || msg.From.ID != "")
}

func (msg *ChatMsg) GetChatMsg() *ChatMsg {
	return msg
}

// ChatMsger is anything based on a ChatMsg.
type ChatMsger interface {
	NetMsger
	GetChatMsg() *ChatMsg
}

// UserInfo has information on a user.
type UserInfo struct {
	User   EntityInfo `json:"user"`             // user
	Photo  MediaInfo  `json:"photo,omitempty"`  // URL or no photo.
	Values ValuesInfo `json:"values,omitempty"` // user values.
}

// MemberInfo has information on a chat member.
type MemberInfo struct {
	TypeInfo            // member
	Info     UserInfo   `json:"info"`
	Values   ValuesInfo `json:"values,omitempty"` // member values.
}

// EnterMsg is about a member entering a chat.
type EnterMsg struct {
	ChatMsg
	Member MemberInfo `json:"member"` // the member who entered.
}

// LeaveMsg is about a member leaving a chat.
type LeaveMsg struct {
	ChatMsg
	User EntityInfo `json:"user"` // the member who left.
	// The ChatMsg.Message can be a leaving message, if supported/provided.
}

// UserChangedMsg is a msg about a user changing.
type UserChangedMsg struct {
	NetMsg
	User   EntityInfo `json:"user"` // the user who changed.
	Info   UserInfo   `json:"info"` // the changes.
	Myself bool       `json:"myself,omitempty"`
}

// MemberChangedMsg is a msg about a member changing in a chat.
type MemberChangedMsg struct {
	ChatMsg
	User   EntityInfo `json:"user"`   // the user who changed.
	Member MemberInfo `json:"member"` // the updated member.
	// The ChatMsg.Message can be a description/summary, but it's optional.
}

// SubscribeMsg is a msg about yourself subscribing or unsubscribing to a chat.
// Myself is not a MemberInfo because Members includes myself,
// and SubscribeMsg is also reused for leaving, which has no need for MemberInfo.
// See the destination type for the type of chat.
type SubscribeMsg struct {
	ChatMsg
	Subject    MessageInfo  `json:"subject,omitempty"`
	Photo      MediaInfo    `json:"photo,omitempty"`   // URL or no photo.
	Members    []MemberInfo `json:"members,omitempty"` // includes myself on subscribe.
	Myself     EntityInfo   `json:"myself"`            // myself as the new member.
	HistoryURL string       `json:"history,omitempty"` // see SubscriptionStateInfo
}

// TypingMsg a msg for a user typing a message.
// Destination is where they are typing.
type TypingMsg struct {
	ChatMsg           // typing
	Typing  bool      `json:"typing"`
	Expires time.Time `json:"expires,omitempty"` // Optional; when typing state expires.
}

// ConnState represents the connection state.
// Note that ConnectFailed during Connecting or Reconnecting does not
// indicate the end of the connection attempt, a Disconnected does.
// ConnectFailed goes to the conn topic, not error;
// it is a ConnMsg and is not an exceptional error.
type ConnState string

const (
	Connecting    ConnState = "connecting"
	Connected     ConnState = "connected"
	Reconnecting  ConnState = "reconnecting"
	Disconnected  ConnState = "disconnected"
	ConnectFailed ConnState = "failed"
)

// ConnMsg is a msg on a connection; also used for network online/offline.
// An implementation is not required to use all the ConnState values,
// but at least Connected and Disconnected are needed.
// Note: the value of Cause may change or may be removed completely!
type ConnMsg struct {
	NetMsg
	Connection EntityInfo `json:"conn"`
	State      ConnState  `json:"state"`
	Cause      string     `json:"cause,omitempty"`
}

func (msg *ConnMsg) Init(id, typ, protocol, netID string, connID string, state ConnState) {
	msg.NetMsg.Init(id, typ, protocol, netID)
	msg.Connection.Init(connID, "conn")
	msg.State = state
}
