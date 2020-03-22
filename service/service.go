package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"stdchat.org"
)

// Networker is a client implementation for a network.
type Networker interface {
	io.Closer
	Receiver
	Logout(reason string) error                 // Same as Close() with logout reason.
	Start(ctx context.Context, id string) error // id = request id
	NetworkID() string
	ConnID() string // empty if no connections.
	Context() context.Context
	Closed() bool
	GetStateInfo() ClientStateInfo
}

// Servicer represents a service.
type Servicer interface {
	io.Closer
	Receiver
	GenericError(err error) // An error from outside, such as during msg dispatch.
	GetClients() []Networker
	GetClientByNetwork(networkID string) Networker
	Protocol() string
	Context() context.Context
	Closed() bool
	GetStateInfo() ServiceStateInfo
}

// Receiver can receive input messages and commands.
type Receiver interface {
	Handler(msg *stdchat.ChatMsg)
	CmdHandler(msg *stdchat.CmdMsg)
}

type NewClientFunc = func(svc *Service, remote, userID, auth string, values stdchat.ValuesInfo) (Networker, error)

// Service is a service.
type Service struct {
	tp          Transporter
	done        chan struct{}
	clients     []Networker // locked by mx
	newClient   NewClientFunc
	mx          sync.RWMutex
	newClientMx sync.Mutex
	closed      int32 // atomic
	Verbose     bool  // verbose output to log.Print/Printf
}

var _ Servicer = &Service{}

// NewService creates a new service.
// newClient must be set to a function, a lock will be acquired during newClient.
// The client eventually needs to call OnClientClosed when done.
func NewService(tp Transporter, newClient NewClientFunc) *Service {
	if tp == nil {
		panic("nil Transporter")
	}
	if newClient == nil {
		panic("nil newClient")
	}
	return &Service{
		tp:        tp,
		done:      make(chan struct{}),
		newClient: newClient,
	}
}

func (svc *Service) Protocol() string {
	return svc.tp.GetProtocol()
}

func (svc *Service) Transporter() Transporter {
	return svc.tp
}

func (svc *Service) GenericError(err error) {
	svc.tp.PublishError("", "", err)
}

func (svc *Service) GetClients() []Networker {
	svc.mx.RLock()
	defer svc.mx.RUnlock()
	return append([]Networker(nil), svc.clients...)
}

func (svc *Service) GetClientByNetwork(networkID string) Networker {
	svc.mx.RLock()
	defer svc.mx.RUnlock()
	for _, client := range svc.clients {
		if client.NetworkID() == networkID {
			return client
		}
	}
	return nil
}

func (svc *Service) Closed() bool {
	return atomic.LoadInt32(&svc.closed) != 0
}

func (svc *Service) Close() error {
	if !atomic.CompareAndSwapInt32(&svc.closed, 0, 1) {
		return errors.New("already closed")
	}
	for _, client := range svc.GetClients() {
		err := client.Close()
		if err != nil {
			return err
		}
	}
	for _, client := range svc.GetClients() {
		<-client.Context().Done()
	}
	close(svc.done)
	return nil
}

type doneCtx struct {
	context.Context
	done <-chan struct{}
}

func (ctx *doneCtx) Done() <-chan struct{} {
	return ctx.done
}

func (ctx *doneCtx) Err() error {
	select {
	case <-ctx.done:
		return errors.New("Service is done")
	default:
		return nil
	}
}

func (ctx *doneCtx) String() string {
	return "Service context"
}

func (svc *Service) Context() context.Context {
	return &doneCtx{context.Background(), svc.done}
}

// addClient adds a client to this service.
// Returns an error if the service is closed or the network ID is in use.
func (svc *Service) addClient(client Networker) error {
	svc.mx.Lock()
	defer svc.mx.Unlock()
	if svc.Closed() {
		return errors.New("service is closed")
	}
	for _, client := range svc.clients {
		if client.NetworkID() == client.NetworkID() {
			return errors.New("network ID is in use")
		}
	}
	svc.clients = append(svc.clients, client)
	return nil
}

// RemoveClient removes the client from this service.
func (svc *Service) removeClient(client Networker) {
	svc.mx.Lock()
	defer svc.mx.Unlock()
	for i, xc := range svc.clients {
		if xc == client {
			ilast := len(svc.clients) - 1
			svc.clients[i], svc.clients[ilast] = svc.clients[ilast], nil
			svc.clients = svc.clients[:ilast]
			break
		}
	}
}

// OnClientClosed is to be called by the Client implementation when done.
// Panics if client.Closed() returns false.
func (svc *Service) OnClientClosed(client Networker) {
	if !client.Closed() {
		panic("client not closed")
	}
	svc.removeClient(client)
}

func (svc *Service) cmdErr(msg *stdchat.CmdMsg, errmsg string) {
	svc.tp.PublishError(msg.ID, msg.Network.ID,
		errors.New("command "+msg.Command+" error: "+errmsg))
}

// CheckArgs ensures the CmdMsg has at least n args, if so returns true;
// otherwise returns false and publishes an error.
func (svc *Service) CheckArgs(n int, msg *stdchat.CmdMsg) bool {
	if len(msg.Args) < n {
		svc.cmdErr(msg, fmt.Sprintf("expected %d args", n))
		return false
	}
	return true
}

func (svc *Service) Login(remote, userID, auth string, values stdchat.ValuesInfo, id string) (Networker, error) {
	var client Networker
	err := func() error {
		svc.newClientMx.Lock()
		defer svc.newClientMx.Unlock()
		var err error
		client, err = svc.newClient(svc, remote, userID, auth, values)
		return err
	}()
	if err != nil {
		return nil, err
	}
	err = svc.addClient(client)
	if err != nil {
		return nil, err
	}
	err = client.Start(client.Context(), id)
	if err != nil {
		svc.removeClient(client)
		return nil, err
	}
	return client, nil
}

func (svc *Service) cmdLogin(remote, userID, auth string, msg *stdchat.CmdMsg) {
	_, err := svc.Login(remote, userID, auth, msg.Values, msg.ID)
	if err != nil {
		svc.cmdErr(msg, err.Error())
	}
}

func (svc *Service) findLogoutID(logoutID string) Networker {
	svc.mx.RLock()
	defer svc.mx.RUnlock()
	for _, client := range svc.clients {
		if client.NetworkID() == logoutID || client.ConnID() == logoutID {
			return client
		}
	}
	return nil
}

func (svc *Service) Logout(logoutID, reason string, values stdchat.ValuesInfo, id string) error {
	client := svc.findLogoutID(logoutID)
	if client == nil {
		return errors.New("unable to logout " + logoutID + " ID not found")
	}
	return client.Logout(reason)
}

func (svc *Service) cmdLogout(logoutID, reason string, msg *stdchat.CmdMsg) {
	err := svc.Logout(logoutID, reason, msg.Values, msg.ID)
	if err != nil {
		svc.cmdErr(msg, err.Error())
	}
}

func (svc *Service) CmdHandler(msg *stdchat.CmdMsg) {
	// Forward to network if network ID present.
	if msg.Network.ID != "" {
		client := svc.GetClientByNetwork(msg.Network.ID)
		if client == nil {
			svc.tp.PublishError(MakeID(msg.ID), "",
				errors.New("network not found: "+msg.Network.ID))
		} else {
			client.CmdHandler(msg)
		}
		return
	}

	switch msg.Command {
	case "login":
		if svc.CheckArgs(3, msg) {
			svc.cmdLogin(msg.Args[0], msg.Args[1], msg.Args[2], msg)
		}
	case "logout":
		if svc.CheckArgs(1, msg) {
			reason := "Logout"
			if len(msg.Args) > 1 {
				reason = msg.Args[1]
			}
			svc.cmdLogout(msg.Args[0], reason, msg)
		}
	case "ping":
		outmsg := &stdchat.BaseMsg{}
		outmsg.Init(MakeID(msg.ID), "other/ping", "") // no protocol
		if len(msg.Args) > 0 {
			outmsg.Message.SetText(msg.Args[0])
		}
		svc.tp.Publish("", "", "other", outmsg)
	default:
		svc.tp.PublishError(msg.ID, msg.Network.ID,
			errors.New("unhandled command: "+msg.Command))
	}
}

func (svc *Service) Handler(msg *stdchat.ChatMsg) {
	if msg.Type == "" || msg.Network.ID == "" {
		svc.tp.PublishError(MakeID(msg.ID), "",
			errors.New("invalid message"))
	} else {
		client := svc.GetClientByNetwork(msg.Network.ID)
		if client == nil {
			svc.tp.PublishError(MakeID(msg.ID), "",
				errors.New("network not found: "+msg.Network.ID))
		} else {
			client.Handler(msg)
		}
	}
}

type ServiceStateInfo struct {
	Protocol      stdchat.ProtocolStateInfo
	Networks      []stdchat.NetworkStateInfo
	Subscriptions []stdchat.SubscriptionStateInfo
}

type ClientStateInfo struct {
	Network       stdchat.NetworkStateInfo
	Subscriptions []stdchat.SubscriptionStateInfo
}

func (svc *Service) GetStateInfo() ServiceStateInfo {
	msg := ServiceStateInfo{}
	msg.Protocol.Type = "proto-state"
	msg.Protocol.Protocol = svc.Protocol()
	for _, client := range svc.GetClients() {
		cstate := client.GetStateInfo()
		msg.Networks = append(msg.Networks, cstate.Network)
		msg.Subscriptions = append(msg.Subscriptions, cstate.Subscriptions...)
	}
	return msg
}

// DispatchMsg dispatches a raw input message to the receiver (service)
func DispatchMsg(rcv Receiver, rawMsg []byte) error {
	if bytes.Index(rawMsg, []byte(`"cmd`)) != -1 {
		msg := &stdchat.CmdMsg{}
		err := stdchat.JSON.Unmarshal(rawMsg, msg)
		if err != nil {
			return err
		}
		if msg.IsType("cmd") {
			rcv.CmdHandler(msg)
			return nil
		}
		// Not cmd, must have found it elsewhere in the payload.
		// Continue to load as ChatMsg...
	}
	msg := &stdchat.ChatMsg{}
	err := stdchat.JSON.Unmarshal(rawMsg, msg)
	if err != nil {
		return err
	}
	rcv.Handler(msg)
	return nil
}
