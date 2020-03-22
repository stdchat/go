// Package provider is a helper package for writing a service provider.
// Note: this provider sub package is experimental and can change at any time!
package provider

import (
	"context"
	"flag"
	"io"
	"net"
	"os"

	"github.com/millerlogic/server-go"
	"stdchat.org/service"
)

// Options for serving this provider.
type Options struct {
	Addr string
	//Password     string
	MaxConns int
	//AutoPassword bool
	AutoExit bool
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
}

func Serve(ln net.Listener, opts Options, svc service.Servicer) error {
	srv := newServer(svc, opts)
	return srv.Serve(ln)
}

func ListenAndServe(opts Options, svc service.Servicer) error {
	srv := newServer(svc, opts)
	srv.Addr = opts.Addr
	return srv.ListenAndServe()
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
		stdio := &struct {
			io.Reader
			io.WriteCloser
		}{os.Stdin, os.Stdout}
		os.Stdout = os.Stderr // Anything going to Go's os.Stdout will go to stderr.
		err := Serve(server.ListenIO(stdio), opts, svc)
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
