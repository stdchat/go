// Package provider is a helper package for writing a service provider.
// Note: this provider sub package is experimental and can change at any time!
package provider

import (
	"context"
	"errors"
	"flag"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/millerlogic/server-go"
	"github.com/millerlogic/server-go/wslisten"
	"stdchat.org/service"
)

// Options for serving this provider.
type Options struct {
	Addr string
	//Password     string
	MaxConns int
	//AutoPassword bool
	AutoExit bool

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
	//flags.StringVar(&opts.Password, "password", opts.Password,
	//	"Set a password, required to use the service")
	flags.IntVar(&opts.MaxConns, "maxConns", opts.MaxConns,
		"Maximum provider connections (when applicable)")
	//flags.BoolVar(&opts.AutoPassword, "autoPassword", opts.AutoPassword,
	//	"Automatic password (first conn sets if not set yet)")
	flags.BoolVar(&opts.AutoExit, "autoExit", opts.AutoExit,
		"Automatically exit upon the last disconnection")

	flags.StringVar(&opts.CertPath, "cert", opts.CertPath,
		"Path to TLS certificate file")
	flags.StringVar(&opts.PrivateKeyPath, "privkey", opts.PrivateKeyPath,
		"Path to TLS private key file")
}

// Serve will serve on the provided listener and options.
// Does not consider TLS (cert & privkey ignored)
func Serve(ln net.Listener, opts Options, svc service.Servicer) error {
	srv := newServer(svc, opts)
	return srv.Serve(ln)
}

func ListenAndServe(opts Options, svc service.Servicer) error {
	srv := newServer(svc, opts)
	srv.Addr = opts.Addr
	if opts.useTLS() {
		return srv.ListenAndServeTLS(opts.CertPath, opts.PrivateKeyPath)
	} else {
		return srv.ListenAndServe()
	}
}

// ListenAndServeWS listens and serves a websocket.
func ListenAndServeWS(opts Options, svc service.Servicer) error {
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

	srv := newServer(svc, opts)

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

func newServer(svc service.Servicer, opts Options) *server.Server {
	if opts.MaxConns == 0 {
		opts.MaxConns = 1
	}
	var srv *server.Server
	srv = &server.Server{
		BaseContext: func(net.Listener) context.Context {
			return svc.Context()
		},
		NewConn: func(ctx context.Context, conn net.Conn) context.Context {
			if opts.MaxConns == 1 && opts.AutoExit {
				srv.MaxConns = -1 // No new conns after this.
			}
			return ctx
		},
		Handler: server.HandlerFunc(func(conn net.Conn, r *server.Request) {
			if len(r.Data) > 0 {
				if err := service.DispatchMsg(svc, r.Data); err != nil {
					svc.GenericError(err)
				}
			}
		}),
		ConnClosed: func(ctx context.Context, conn net.Conn, err error) {
			if err != nil {
				svc.GenericError(err)
			}
			if srv.NumConns() == 0 && opts.AutoExit {
				srv.Close() // Auto exit.
				svc.Close()
			}
		},
		MaxConns: opts.MaxConns,
	}
	return srv
}

// Run is a convenience function to run an entire provider program.
func Run(protocol string, newService func(t service.Transporter) service.Servicer) error {
	opts := DefaultOptions
	opts.AddFlags(flag.CommandLine)
	flag.Parse()

	t := &service.LocalTransport{
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
		err := Serve(server.ListenIO(stdio), opts, svc)
		if err != nil && err != server.ErrServerClosed {
			return err
		}
	} else if strings.HasPrefix(opts.Addr, "ws:") || strings.HasPrefix(opts.Addr, "wss:") {
		err := ListenAndServeWS(opts, svc)
		if err != nil && err != server.ErrServerClosed {
			return err
		}
	} else {
		err := ListenAndServe(opts, svc)
		if err != nil && err != server.ErrServerClosed {
			return err
		}
	}
	return nil
}
