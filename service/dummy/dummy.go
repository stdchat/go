package dummy

import (
	"context"
	"errors"
	"strings"

	"stdchat.org"
	"stdchat.org/service"
)

func NewService(tp service.Transporter) *service.Service {
	return service.NewService(tp, NewClient)
}

func NewClient(svc *service.Service, addr, nick, pass string, values stdchat.ValuesInfo) (service.Networker, error) {
	if svc.Closed() {
		return nil, errors.New("service is closed")
	}

	client := &Client{
		svc:   svc,
		tp:    svc.Transporter(),
		users: make(map[string]string),
	}
	client.users[client.UserID()] = client.UserName()
	client.ctx, client.ctxCancel = context.WithCancel(context.Background())

	return client, nil
}

const Protocol = "dummy"

type Client struct {
	svc       *service.Service
	tp        service.Transporter
	ctx       context.Context
	users     map[string]string // map of userID to userName
	ctxCancel func()
}

func (client *Client) getUser(x string) (userID, userName string) {
	lx := strings.ToLower(x)
	if userName, ok := client.users[lx]; ok {
		return lx, userName
	}
	client.users[lx] = x
	return lx, x
}

func (client *Client) Close() error {
	select {
	case <-client.ctx.Done():
		// Already closed.
	default:
		client.ctxCancel()
		msg := &stdchat.NetMsg{}
		msg.Init(service.MakeID(""), "offline", Protocol,
			client.NetworkID())
		client.tp.Publish(msg.Network.ID, "", "network", msg)
	}
	return nil
}

func (client *Client) Handler(msg *stdchat.ChatMsg) {
	switch msg.Type {
	case "msg", "msg/dummy.fakeMsg":
		// Send outgoing msg back with all info.
		outmsg := stdchat.ChatMsg{}
		outmsg.Init(service.MakeID(msg.ID), "msg/dummy.fakeMsg", Protocol, client.NetworkID())
		outmsg.From.Init(client.UserID(), "user")
		outmsg.From.SetName(client.UserName(), "")
		destID, destName := client.getUser(msg.Destination.GetName())
		outmsg.Destination.Init(destID, "user")
		outmsg.Destination.SetName(destName, "")
		outmsg.Message.SetText(msg.GetMessageString())
		client.tp.Publish(client.NetworkID(), outmsg.Destination.ID, "msg-out", outmsg)
		// Also have the recipient echo it, for dummy data:
		client.publishFakeMsg(destName, "you said \""+msg.GetMessageString()+"\"")
	//case "msg/action", "msg/action/dummy.fakeAction":
	//case "info", "info/dummy.fakeInfo":
	default:
		client.tp.PublishError(service.MakeID(msg.ID), msg.Network.ID,
			errors.New("unhandled message of type "+msg.Type))
	}
}

func (client *Client) CmdHandler(msg *stdchat.CmdMsg) {
	client.tp.PublishError(msg.ID, msg.Network.ID,
		errors.New("unhandled command: "+msg.Command))
}

func (client *Client) Logout(reason string) error {
	return client.Close()
}

func (client *Client) publishFakeMsg(from, msgText string) {
	fromID, fromName := client.getUser(from)
	msg := stdchat.ChatMsg{}
	msg.Init(service.MakeID(""), "msg/dummy.fakeMsg", Protocol, client.NetworkID())
	msg.From.Init(fromID, "user")
	msg.From.SetName(fromName, "")
	msg.Destination = msg.From
	msg.Message.SetText(msgText)
	client.tp.Publish(client.NetworkID(), msg.Destination.ID, "msg", msg)
}

func (client *Client) Start(ctx context.Context, id string) error {
	msg := &stdchat.NetMsg{}
	msg.Init(service.MakeID(id), "online", Protocol,
		client.NetworkID())
	client.tp.Publish(msg.Network.ID, "", "network", msg)
	client.publishFakeMsg("FakeUser", "hello")
	return nil
}

func (client *Client) NetworkID() string {
	return "dummy"
}

func (client *Client) NetworkName() string {
	return "Dummy"
}

func (client *Client) ConnID() string {
	return "" // not applicable
}

func (client *Client) Context() context.Context {
	return client.ctx
}

func (client *Client) Closed() bool {
	select {
	case <-client.ctx.Done():
		return true
	default:
		return false
	}
}

func (client *Client) UserID() string {
	return "myself"
}

func (client *Client) UserName() string {
	return "Myself"
}

func (client *Client) GetStateInfo() service.ClientStateInfo {
	msg := stdchat.NetworkStateInfo{}
	msg.Type = "network-state"
	msg.Ready = true
	msg.Network.Init(client.NetworkID(), "net")
	msg.Network.SetName(client.NetworkName(), "")
	msg.Myself.Init(client.UserID(), "user")
	msg.Myself.SetName(client.UserName(), "")
	msg.Protocol = Protocol
	return service.ClientStateInfo{
		Network: msg,
	}
}
