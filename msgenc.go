package stdchat

import "errors"

// ParseBaseMsg parses rawMsg JSON into a specific base msg type.
func ParseBaseMsg(rawMsg []byte) (BaseMsger, error) {
	msg := &ChatMsg{}
	err := json.Unmarshal(rawMsg, msg)
	if err != nil {
		return nil, err
	}
	switch {
	case msg.IsType("enter"):
		return reparseBaseMsg(&EnterMsg{}, rawMsg)
	case msg.IsType("leave"):
		return reparseBaseMsg(&LeaveMsg{}, rawMsg)
	case msg.IsType("user-changed"):
		return reparseBaseMsg(&UserChangedMsg{}, rawMsg)
	case msg.IsType("member-changed"):
		return reparseBaseMsg(&MemberChangedMsg{}, rawMsg)
	case msg.IsType("subscribe") || msg.IsType("unsubscribe"):
		return reparseBaseMsg(&SubscribeMsg{}, rawMsg)
	case msg.IsType("typing"):
		return reparseBaseMsg(&TypingMsg{}, rawMsg)
	case msg.IsType("conn-state"):
		return reparseBaseMsg(&ConnMsg{}, rawMsg)
	case msg.IsType("cmd"):
		return reparseBaseMsg(&CmdMsg{}, rawMsg)
	default: // Default rules:
		if msg.IsMsg() {
			return msg, nil
		}
		if msg.BaseMsg.IsMsg() {
			if msg.Network.ID != "" {
				return &NetMsg{BaseMsg: msg.BaseMsg, Network: msg.Network}, nil
			}
			return &msg.BaseMsg, nil
		}
		return msg, errors.New("not a valid message")
	}
}

func reparseBaseMsg(msg BaseMsger, rawMsg []byte) (BaseMsger, error) {
	err := json.Unmarshal(rawMsg, msg)
	if err != nil {
		return nil, err
	}
	if !msg.IsMsg() {
		return msg, errors.New("not a valid message")
	}
	return msg, nil
}
