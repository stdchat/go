package service

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"

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

type MultiTransporter interface {
	Transporter
	AddTransport(transport Transporter)
	RemoveTransport(transport Transporter)
}

// MultiTransport relays messages to zero or more other transports.
// It is thread safe.
type MultiTransport struct {
	Protocol string
	WebServer
	mx         sync.RWMutex
	transports []Transporter
}

var _ MultiTransporter = &MultiTransport{}

func (tp *MultiTransport) AddTransport(transport Transporter) {
	tp.mx.Lock()
	defer tp.mx.Unlock()
	tp.transports = append(tp.transports, transport)
}

func (tp *MultiTransport) RemoveTransport(transport Transporter) {
	tp.mx.Lock()
	defer tp.mx.Unlock()
	for i, tx := range tp.transports {
		if tx == transport {
			tp.transports = append(tp.transports[:i], tp.transports[i+1:]...)
			break
		}
	}
}

func (tp *MultiTransport) Close() error {
	tp.mx.Lock()
	defer tp.mx.Unlock()
	var mec multiTpErrorCollector
	for _, tx := range tp.transports {
		err := tx.Close()
		if err != nil {
			mec.Add(tx, err)
		}
	}
	tp.transports = nil
	err := tp.WebServer.Close()
	if err != nil {
		mec.Add(tp, err)
	}
	return mec.GetError()
}

func (tp *MultiTransport) GetProtocol() string {
	return tp.Protocol
}

func (tp *MultiTransport) Advertise() error {
	tp.mx.RLock()
	defer tp.mx.RUnlock()
	if tp.Protocol == "" {
		if len(tp.transports) != 0 {
			tp.Protocol = tp.transports[0].GetProtocol()
		} else {
			tp.Protocol = "protocol"
		}
	}
	return nil
}

// Publish will be called on all the added transports,
// errors will be collected into a *MultiTransportError if more than one error,
// *SingleTransportError if just one error, or nil if no errors.
func (tp *MultiTransport) Publish(network, chat, node string, payload interface{}) error {
	tp.mx.RLock()
	defer tp.mx.RUnlock()
	var mec multiTpErrorCollector
	for _, tx := range tp.transports {
		err := tx.Publish(network, chat, node, payload)
		if err != nil {
			mec.Add(tx, err)
		}
	}
	return mec.GetError()
}

func (tp *MultiTransport) PublishError(id string, network string, err error) error {
	msg := &stdchat.NetMsg{}
	msg.Init(id, "error", tp.Protocol, network)
	msg.Message.SetText(err.Error())
	return tp.Publish(network, "", "error", msg)
}

type SingleTransportError struct {
	Transport Transporter
	Err       error
}

func (err *SingleTransportError) Error() string {
	return err.Err.Error()
}

func (err *SingleTransportError) Unwrap() error {
	return err.Err
}

type MultiTransportError struct {
	Errors []SingleTransportError
}

func (err *MultiTransportError) Error() string {
	msg := err.Errors[0].Err.Error()
	if len(err.Errors) > 1 {
		msg = fmt.Sprintf("%s (and %d more)", msg, len(err.Errors)-1)
	}
	return msg
}

/*func (err *MultiTransportError) Unwrap() error {
	return err.Errors[0].Err
}*/

type multiTpErrorCollector struct {
	sterr SingleTransportError
	mterr MultiTransportError
}

func (mec *multiTpErrorCollector) Add(tx Transporter, err error) {
	thiserr := SingleTransportError{
		Transport: tx,
		Err:       err,
	}
	if mec.mterr.Errors != nil {
		// Already multiple errors, append.
		mec.mterr.Errors = append(mec.mterr.Errors, thiserr)
	} else if mec.sterr.Err != nil {
		// Second error, put both in mterr.
		mec.mterr.Errors = append(mec.mterr.Errors, mec.sterr, thiserr)
	} else {
		// First error, use sterr.
		mec.sterr = thiserr
	}
}

func (mec *multiTpErrorCollector) GetError() error {
	if mec.mterr.Errors != nil {
		return &mec.mterr
	}
	if mec.sterr.Err != nil {
		return &mec.sterr
	}
	return nil
}
