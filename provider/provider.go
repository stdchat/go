// Package provider is a helper package for writing a service provider.
// Note: this provider sub package is experimental and can change at any time!
package provider

import (
	"context"
	"errors"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/millerlogic/server-go"
	"github.com/millerlogic/server-go/wslisten"
	"stdchat.org"
	"stdchat.org/service"
)

// Options for serving this provider.
type Options struct {
	Addr         string
	Password     string
	MaxConns     int
	AutoPassword bool
	AutoExit     bool

	// TLS:
	CertPath, PrivateKeyPath string
}

func (opts *Options) useTLS() bool {
	return opts.CertPath != "" || opts.PrivateKeyPath != ""
}

// DefaultOptions - do not modify, make a copy before changing or calling AddFlags.
var DefaultOptions = Options{
	MaxConns: 1,
	//AutoPassword: true,
	AutoExit: true,
}

// AddFlags adds flags for the options.
func (opts *Options) AddFlags(flags *flag.FlagSet) {
	flags.StringVar(&opts.Addr, "addr", opts.Addr,
		"Set the listen address for the provider")
	flags.StringVar(&opts.Password, "password", opts.Password,
		"Set a password, required to use the service")
	flags.IntVar(&opts.MaxConns, "maxConns", opts.MaxConns,
		"Maximum provider connections (when applicable)")
	flags.BoolVar(&opts.AutoPassword, "autoPassword", opts.AutoPassword,
		"Automatic password (first conn sets if not set yet)")
	flags.BoolVar(&opts.AutoExit, "autoExit", opts.AutoExit,
		"Automatically exit upon the last disconnection")

	flags.StringVar(&opts.CertPath, "cert", opts.CertPath,
		"Path to TLS certificate file")
	flags.StringVar(&opts.PrivateKeyPath, "privkey", opts.PrivateKeyPath,
		"Path to TLS private key file")
}

// Serve will serve on the provided listener and options.
// Does not consider TLS (cert & privkey ignored)
func Serve(ln net.Listener, opts Options, svc service.Servicer, tp service.MultiTransporter) error {
	srv := newProvider(opts, svc, tp)
	return srv.Serve(ln)
}

func ListenAndServe(opts Options, svc service.Servicer, tp service.MultiTransporter) error {
	srv := newProvider(opts, svc, tp)
	srv.Addr = opts.Addr
	if opts.useTLS() {
		return srv.ListenAndServeTLS(opts.CertPath, opts.PrivateKeyPath)
	} else {
		return srv.ListenAndServe()
	}
}

// ListenAndServeWS listens and serves a websocket.
func ListenAndServeWS(opts Options, svc service.Servicer, tp service.MultiTransporter) error {
	if opts.MaxConns == 0 {
		opts.MaxConns = 1
	}

	u, err := url.Parse(opts.Addr)
	if err != nil {
		return err
	}

	switch u.Scheme {
	case "ws":
		if opts.useTLS() {
			return errors.New("expected wss:// url")
		}
	case "wss":
		if !opts.useTLS() {
			return errors.New("expected cert and private key for wss://")
		}
	default:
		return errors.New("not a valid websocket addr")
	}

	mux := &http.ServeMux{}
	httpserver := &http.Server{
		Addr:    u.Host,
		Handler: mux,
	}
	wsln := wslisten.ListenWS()
	wsln.DefaultFormat = wslisten.TextFormat

	pattern := u.Path
	if pattern == "" {
		pattern = "/"
	}
	mux.Handle(pattern, wsln)

	srv := newProvider(opts, svc, tp)

	httpch := make(chan struct{})
	var httpErr error
	go func() {
		defer close(httpch)
		if opts.useTLS() {
			httpErr = httpserver.ListenAndServeTLS(opts.CertPath, opts.PrivateKeyPath)
		} else {
			httpErr = httpserver.ListenAndServe()
		}
		srv.Close()
		wsln.Close()
	}()

	srvErr := srv.Serve(wsln)

	httpserver.Close()
	<-httpch
	if httpErr != nil && httpErr != http.ErrServerClosed {
		return httpErr
	}
	return srvErr
}

type ctxKey struct{ name string }

func (x *ctxKey) String() string { return x.name }

type provider struct {
	*server.Server
	opts             Options // readonly
	mx               sync.RWMutex
	password         string // locked by mx (in case of AutoPassword update)
	passwordDisabled bool
}

// if opts.AutoPassword is true and the password hasn't been set yet,
// this function sets the password.
// Returns false if wrong password.
func (p *provider) PasswordCheck(pw string) bool {
	p.mx.Lock()
	defer p.mx.Unlock()
	if p.passwordDisabled {
		return false
	}
	if pw == p.password {
		return true
	}
	if p.password == "" && p.opts.AutoPassword {
		p.password = pw
		return true
	}
	return false
}

// The user is allowed to skip past provider auth in one particular case.
// It allows the first connection to ignore auth and lock out other connections.
func (p *provider) PasswordCheckSkip() bool {
	p.mx.Lock()
	defer p.mx.Unlock()
	if !p.opts.AutoPassword {
		return false
	}
	if p.passwordDisabled || p.password != "" {
		return false
	}
	p.passwordDisabled = true
	return true
}

type clientInfo struct {
	p      *provider
	tp     *connTransport // only added to the multi tp if authed.
	authed bool
}

var clientInfoKey = &ctxKey{"*clientInfo"}

func newProvider(opts Options, svc service.Servicer, tp service.MultiTransporter) *provider {
	if opts.MaxConns == 0 {
		opts.MaxConns = 1
	}
	p := &provider{
		opts:     opts,
		password: opts.Password,
	}
	var srv *server.Server
	srv = &server.Server{
		BaseContext: func(net.Listener) context.Context {
			return svc.Context()
		},
		NewConn: func(ctx context.Context, conn net.Conn) context.Context {
			wantServiceAuth := opts.AutoPassword || opts.Password != ""
			cinfo := &clientInfo{
				p:      p,
				tp:     &connTransport{conn: conn},
				authed: !wantServiceAuth,
			}
			err := cinfo.tp.Advertise()
			if err != nil {
				log.Printf("transport advertise error: %v", err)
			}
			ctx = context.WithValue(ctx, clientInfoKey, cinfo)
			if opts.MaxConns == 1 && opts.AutoExit {
				srv.MaxConns = -1 // No new conns after this.
			}
			if cinfo.authed {
				tp.AddTransport(cinfo.tp)
			}
			return ctx
		},
		Handler: server.HandlerFunc(func(conn net.Conn, r *server.Request) {
			if len(r.Data) > 0 {
				cinfo, _ := r.Context().Value(clientInfoKey).(*clientInfo)
				if cinfo == nil {
					log.Println("provider Handler ctx does not contain clientInfoKey")
					return
				}
				if !cinfo.authed { // Not authed yet.
					// Note: while not authed, any responses (including errors)
					// should go to cinfo.tp directly, NOT tp or svc.GenericError!
					msg := &stdchat.CmdMsg{}
					if err := stdchat.DecodeMsg(r.Data, msg); err != nil {
						cinfo.tp.PublishError(msg.ID, msg.Network.ID, err)
						return
					}
					if msg.IsType("cmd") && msg.Command == "provider-auth" {
						if len(msg.Args) < 1 {
							err := errors.New("unexpected command args")
							cinfo.tp.PublishError(msg.ID, msg.Network.ID, err)
							return
						}
						if msg.Network.ID != "" {
							err := errors.New("cannot provider-auth to a network")
							cinfo.tp.PublishError(msg.ID, msg.Network.ID, err)
							return
						}
						if !cinfo.p.PasswordCheck(msg.Args[0]) {
							err := errors.New("authentication failed")
							cinfo.tp.PublishError(msg.ID, msg.Network.ID, err)
							return
						}
						cinfo.authed = true
						tp.AddTransport(cinfo.tp)
						outmsg := &stdchat.BaseMsg{}
						outmsg.Init(msg.ID, "", tp.GetProtocol())
						outmsg.Message.SetText("authenticated")
						cinfo.tp.Publish(msg.Network.ID, "", "info/provider.auth", &outmsg)
						return
					} else if cinfo.p.PasswordCheckSkip() {
						cinfo.authed = true
						tp.AddTransport(cinfo.tp)
						// Fall through and process the current message.
					} else {
						err := errors.New("must authenticate with the provider first (provider-auth)")
						cinfo.tp.PublishError(msg.ID, msg.Network.ID, err)
						return
					}
				}
				if err := service.DispatchMsg(svc, r.Data); err != nil {
					svc.GenericError(err)
					return
				}
			}
		}),
		ConnClosed: func(ctx context.Context, conn net.Conn, err error) {
			if err != nil {
				svc.GenericError(err)
			}
			cinfo, _ := ctx.Value(clientInfoKey).(*clientInfo)
			if cinfo == nil {
				log.Println("provider ConnClosed ctx does not contain clientInfoKey")
			} else {
				tp.RemoveTransport(cinfo.tp)
			}
			if srv.NumConns() == 0 && opts.AutoExit {
				srv.Close() // Auto exit.
				svc.Close()
			}
		},
		MaxConns: opts.MaxConns,
	}
	p.Server = srv
	return p
}

// Run is a convenience function to run an entire provider program.
func Run(protocol string, newService func(t service.Transporter) service.Servicer) error {
	opts := DefaultOptions
	opts.AddFlags(flag.CommandLine)
	flag.Parse()

	t := &service.MultiTransport{
		Protocol: protocol,
	}
	svc := newService(t)
	err := t.Advertise()
	if err != nil {
		return err
	}

	if opts.Addr == "" || opts.Addr == "-" {
		if opts.useTLS() {
			return errors.New("Do not use cert/privkey with standard I/O")
		}
		stdio := &struct {
			io.Reader
			io.WriteCloser
		}{os.Stdin, os.Stdout}
		os.Stdout = os.Stderr // Anything going to Go's os.Stdout will go to stderr.
		err := Serve(server.ListenIO(stdio), opts, svc, t)
		if err != nil && err != server.ErrServerClosed {
			return err
		}
	} else if strings.HasPrefix(opts.Addr, "ws:") || strings.HasPrefix(opts.Addr, "wss:") {
		err := ListenAndServeWS(opts, svc, t)
		if err != nil && err != server.ErrServerClosed {
			return err
		}
	} else {
		err := ListenAndServe(opts, svc, t)
		if err != nil && err != server.ErrServerClosed {
			return err
		}
	}
	return nil
}

type connTransport struct {
	service.LocalTransport
	conn net.Conn
}

func (tp *connTransport) Advertise() error {
	err := tp.LocalTransport.Advertise()
	if err != nil {
		return err
	}
	tp.PublishHandler = func(_ *service.LocalTransport,
		network, chat, node string, payload interface{}) error {
		return tp.publish(network, chat, node, payload)
	}
	return nil
}

func (tp *connTransport) publish(network, chat, node string, payload interface{}) error {
	j, err := stdchat.JSON.Marshal(&struct {
		Node    string      `json:"node"`
		Payload interface{} `json:"payload"`
	}{node, payload})
	if err != nil {
		return err
	}
	_, err = tp.conn.Write(append(j, '\n'))
	return err
}
