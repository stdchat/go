package service

import (
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

type WebServer struct {
	PublicURL string // uses localhost and a random port if empty.
	BindAddr  string // parses from PublicURL if empty.
	srv       http.Server
	mx        sync.Mutex
	mux       mymux // locked by mx
}

func (ws *WebServer) ServeURL(network, pathSuffix string, handler http.Handler) (string, error) {
	ws.mx.Lock()
	defer ws.mx.Unlock()

	isFirst := ws.mux.NumHandlers() == 0
	vurl := fmt.Sprintf("/%s/%s", network, pathSuffix)
	ws.mux.Handle(vurl, handler)

	if isFirst && ws.mux.NumHandlers() == 1 { // First handler, setup the server.

		//ws.srv.Handler = &ws.mux // Not locked.
		ws.srv.Handler = ws

		bindAddr := ws.BindAddr
		if bindAddr == "" && ws.PublicURL != "" {
			u, err := url.Parse(ws.PublicURL)
			if err != nil {
				return "", err
			}
			bindAddr = u.Host
		}

		var listener net.Listener
		for itry := 0; ; itry++ {
			ws.srv.Addr = bindAddr
			if bindAddr == "" {
				// TODO: bind to :0 and get address.
				ws.srv.Addr = fmt.Sprintf("localhost:%d", 10000+rand.Intn(50000))
			}
			var err error
			listener, err = net.Listen("tcp", ws.srv.Addr)
			if err == nil {
				break
			}
			if itry > 10 || bindAddr != "" {
				return "", err
			}
		}

		if ws.PublicURL == "" {
			if ws.srv.Addr[0] == ':' {
				ws.PublicURL = "http://localhost" + ws.srv.Addr
			} else {
				ws.PublicURL = "http://" + ws.srv.Addr
			}
		}

		go func() {
			err := ws.srv.Serve(listener)
			if err != nil && err != http.ErrServerClosed {
				log.Printf("Error in HTTP Serve: %v", err)
			}
		}()
	}

	return ws.PublicURL + vurl, nil
}

func (ws *WebServer) StopServeURL(network, pathSuffix string) {
	ws.mx.Lock()
	defer ws.mx.Unlock()

	isLast := ws.mux.NumHandlers() == 1
	vurl := fmt.Sprintf("/%s/%s", network, pathSuffix)
	ws.mux.RemoveHandler(vurl)

	if isLast && ws.mux.NumHandlers() == 0 { // Last handler removed, stop server.
		ws.srv.Close()
		ws.srv = http.Server{}
	}
}

func (ws *WebServer) Close() error {
	ws.mx.Lock()
	defer ws.mx.Unlock()

	ws.mux.Close()
	err := ws.srv.Close()
	ws.srv = http.Server{}
	return err
}

func (ws *WebServer) FindHandler(path string) http.Handler {
	ws.mx.Lock()
	defer ws.mx.Unlock()
	return ws.mux.FindHandler(path)
}

func (ws *WebServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s %s", r.RemoteAddr, r.URL.String())
	handler := ws.FindHandler(r.URL.Path)
	if handler != nil {
		handler.ServeHTTP(w, r)
	} else {
		//log.Printf("No handler for %s", r.URL.String())
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
	}
}

type mymux struct {
	handlers map[string]http.Handler // nil until first Handle call
}

func (mux *mymux) NumHandlers() int {
	if mux.handlers == nil {
		return 0
	}
	return len(mux.handlers)
}

func (mux *mymux) Close() error {
	mux.handlers = nil
	return nil
}

func (mux *mymux) Handle(hpath string, handler http.Handler) {
	if handler == nil {
		panic("nil Handler")
	}
	if mux.handlers == nil {
		mux.handlers = make(map[string]http.Handler)
	}
	mux.handlers[hpath] = handler
}

func (mux *mymux) RemoveHandler(hpath string) {
	if mux.handlers == nil {
		return
	}
	delete(mux.handlers, hpath)
}

func (mux *mymux) FindHandler(path string) http.Handler {
	var bestHandler http.Handler
	var bestHandlerLen int
	if mux.handlers != nil {
		// Find the longest matching handler.
		for hpath, handler := range mux.handlers {
			if len(hpath) > bestHandlerLen && strings.HasPrefix(path, hpath) {
				bestHandler = handler
				bestHandlerLen = len(hpath)
			}
		}
	}
	return bestHandler
}

func (mux *mymux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	handler := mux.FindHandler(r.URL.Path)
	if handler != nil {
		handler.ServeHTTP(w, r)
	} else {
		//log.Printf("No handler for %s", r.URL.String())
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
	}
}
