package service

import (
	"io"
	"net/http"
	"os"

	"stdchat.org"
)

// Transporter is a chat service transport.
type Transporter interface {
	io.Closer

	GetProtocol() string

	// Advertise the service is up,
	// this needs to be called before clients can do anything.
	Advertise() error
	// Publish a message.
	// network and chat can be empty.
	// node is the final part in the topic name, as in chat/Protocol/node
	// e.g. to publish a protocol msg, use Publish("", "", "my-info", payload)
	Publish(network, chat, node string, payload interface{}) error

	// id should be the ID of the request (if a request), which can be empty.
	// network can be empty if a protocol msg.
	PublishError(id string, network string, err error) error

	// Full URL returned.
	// pathSuffix is the last part of the URL path, such as userA/foo4.png
	// handler is set to a HTTP client handler to serve the headers+content.
	// It is resonable that a directory is served.
	// Use StopServeURL to stop serving.
	ServeURL(network, pathSuffix string, handler http.Handler) (string, error)

	// Cancel serving a URL.
	// If this is the last served URL, the web service may shut down.
	StopServeURL(network, pathSuffix string)
}

// LocalTransport is a service transport for local use. All fields optional.
type LocalTransport struct {
	Protocol       string
	PublishHandler func(tp *LocalTransport,
		network, chat, node string, payload interface{}) error
	WebServer
}

var _ Transporter = &LocalTransport{}

func (tp *LocalTransport) GetProtocol() string {
	return tp.Protocol
}

func (tp *LocalTransport) Advertise() error {
	if tp.Protocol == "" {
		tp.Protocol = "protocol"
	}
	if tp.PublishHandler == nil {
		tp.PublishHandler = DefaultLocalTransportPublish
	}
	return nil
}

func (tp *LocalTransport) Publish(network, chat, node string, payload interface{}) error {
	return tp.PublishHandler(tp, network, chat, node, payload)
}

func (tp *LocalTransport) PublishError(id string, network string, err error) error {
	msg := &stdchat.NetMsg{}
	msg.Init(id, "error", tp.Protocol, network)
	msg.Message.SetText(err.Error())
	return tp.Publish(network, "", "error", msg)
}

func DefaultLocalTransportPublish(tp *LocalTransport,
	network, chat, node string, payload interface{}) error {
	j, err := stdchat.JSON.Marshal(&struct {
		Node    string      `json:"node"`
		Payload interface{} `json:"payload"`
	}{node, payload})
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(append(j, '\n'))
	return err
}
