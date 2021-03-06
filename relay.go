/*
 * Copyright (c) 2015, Shinya Yagyu
 * All rights reserved.
 * Redistribution and use in source and binary forms, with or without
 * modification, are permitted provided that the following conditions are met:
 *
 * 1. Redistributions of source code must retain the above copyright notice,
 *    this list of conditions and the following disclaimer.
 * 2. Redistributions in binary form must reproduce the above copyright notice,
 *    this list of conditions and the following disclaimer in the documentation
 *    and/or other materials provided with the distribution.
 * 3. Neither the name of the copyright holder nor the names of its
 *    contributors may be used to endorse or promote products derived from this
 *    software without specific prior written permission.
 *
 * THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
 * AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
 * IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
 * ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE
 * LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
 * CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
 * SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
 * INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN
 * CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
 * ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
 * POSSIBILITY OF SUCH DAMAGE.
 */

package relay

import (
	"bytes"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/websocket"
)

//Request is for relaying http.request , which doesn't include ones that cannot be converted to JSON.
type request struct {
	Method           string
	URL              *url.URL
	Proto            string // "HTTP/1.0"
	ProtoMajor       int    // 1
	ProtoMinor       int    // 0
	Header           http.Header
	Body             []byte
	ContentLength    int64
	TransferEncoding []string
	Host             string
	Form             url.Values
	Trailer          http.Header
	RemoteAddr       string
	RequestURI       string
	Error            error
	IsPing           bool
	Close            bool
}

//fromRequest converts http.Request to request.
func fromRequest(r *http.Request, err error) *request {
	re := &request{
		Method:           r.Method,
		URL:              r.URL,
		Proto:            r.Proto,
		ProtoMajor:       r.ProtoMajor,
		ProtoMinor:       r.ProtoMinor,
		Header:           r.Header,
		ContentLength:    r.ContentLength,
		TransferEncoding: r.TransferEncoding,
		Close:            r.Close,
		Host:             r.Host,
		Form:             r.Form,
		Trailer:          r.Trailer,
		RemoteAddr:       r.RemoteAddr,
		RequestURI:       r.RequestURI,
		Error:            err,
	}
	re.Body, err = ioutil.ReadAll(r.Body)
	err2 := r.Body.Close()
	if err != nil {
		re.Error = err
		return re
	}
	if err2 != nil {
		re.Error = err2
	}
	return re
}

//toRequst converts request to http.Request
func (r *request) toRequest() (*http.Request, error) {
	if r.Error != nil {
		return nil, r.Error
	}
	b := bytes.NewReader(r.Body)
	re, err := http.NewRequest(r.Method, r.URL.String(), b)
	if err != nil {
		return nil, err
	}
	re.Proto = r.Proto
	re.ProtoMajor = r.ProtoMajor
	re.ProtoMinor = r.ProtoMinor
	re.Header = r.Header
	re.ContentLength = r.ContentLength
	re.TransferEncoding = r.TransferEncoding
	re.Close = r.Close
	re.Host = r.Host
	re.Form = r.Form
	re.Trailer = r.Trailer
	re.RemoteAddr = r.RemoteAddr
	re.RequestURI = r.RequestURI
	return re, nil
}

//ResponseWriter is simple struct for http.ResponseWriter.
type ResponseWriter struct {
	Head       http.Header
	Body       []byte
	StatusCode int
}

// Header returns the header map that will be sent by
// WriteHeader. Changing the header after a call to
// WriteHeader (or Write) has no effect unless the modified
// headers were declared as trailers by setting the
// "Trailer" header before the call to WriteHeader (see example).
// To suppress implicit response headers, set their value to nil.
func (r *ResponseWriter) Header() http.Header {
	if r.Head == nil {
		r.Head = make(http.Header)
	}
	return r.Head
}

// Write writes the data to the connection as part of an HTTP reply.
// If WriteHeader has not yet been called, Write calls WriteHeader(http.StatusOK)
// before writing the data.  If the Header does not contain a
// Content-Type line, Write adds a Content-Type set to the result of passing
// the initial 512 bytes of written data to DetectContentType.
func (r *ResponseWriter) Write(d []byte) (int, error) {
	r.Body = append(r.Body, d...)
	return len(d), nil
}

// WriteHeader sends an HTTP response header with status code.
// If WriteHeader is not called explicitly, the first call to Write
// will trigger an implicit WriteHeader(http.StatusOK).
// Thus explicit calls to WriteHeader are mainly used to
// send error codes.
func (r *ResponseWriter) WriteHeader(s int) {
	r.StatusCode = s
}

//copyTo copies r to http.ResponseWriter
func (r *ResponseWriter) copyTo(w http.ResponseWriter) error {
	for k, vs := range r.Head {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	if r.StatusCode != 0 {
		w.WriteHeader(r.StatusCode)
	}
	if _, err := w.Write(r.Body); err != nil {
		return err
	}
	return nil
}

var sockets = make(map[string]*wsRelayServer)
var count int32
var mutex sync.RWMutex

type wsRelayServer struct {
	ws   *websocket.Conn
	msg  chan interface{}
	stop chan struct{}
}

//Count returns # of relay clients.
func Count() int32 {
	return atomic.LoadInt32(&count)
}

//IsAccepted retruns true if prefix is already accepted.
func IsAccepted(prefix string) bool {
	mutex.RLock()
	defer mutex.RUnlock()
	for n := range sockets {
		if strings.HasPrefix(n, prefix) {
			return true
		}
	}
	return false
}

//StartServe starts to relay.
//It registers ws connection as name and wait for w.stop channel signal.
func StartServe(name string, ws *websocket.Conn) {
	w := &wsRelayServer{
		ws:   ws,
		msg:  make(chan interface{}),
		stop: make(chan struct{}),
	}
	setDeadlines(ws)

	mutex.Lock()
	if old := sockets[name]; old != nil {
		old.stop <- struct{}{}
	}
	sockets[name] = w
	mutex.Unlock()
	w.writePump()

	<-w.stop
	log.Println("relay exited")
	atomic.AddInt32(&count, -1)
	if err := ws.Close(); err != nil {
		log.Println(err)
	}
	delete(sockets, name)
}

//StopServe stops relaying associated with name.
func StopServe(name string) {
	mutex.RLock()
	defer mutex.RUnlock()
	if w, exist := sockets[name]; exist {
		w.stop <- struct{}{}
	}
}

func (r *wsRelayServer) writePump() {
	go func() {
		for {
			select {
			case <-time.Tick(time.Minute):
				if err := sendPing(r.ws); err != nil {
					log.Println(err)
					r.stop <- struct{}{}
					return
				}
				if err := recvPing(r.ws); err != nil {
					log.Println(err)
					r.stop <- struct{}{}
					return
				}
			case req := <-r.msg:
				if err := websocket.JSON.Send(r.ws, req); err != nil {
					log.Println(err)
					r.stop <- struct{}{}
					return
				}
			}
		}
	}()
}

func recvPing(ws *websocket.Conn) error {
	var req request
	if err := websocket.JSON.Receive(ws, &req); err != nil {
		log.Println(err)
		return err
	}
	if !req.IsPing {
		err := errors.New("not ping")
		log.Println(err)
		return err
	}
	log.Println("pong received")
	return nil
}

func sendPing(ws *websocket.Conn) error {
	log.Println("sendig ping")
	req := request{
		IsPing: true,
	}
	return websocket.JSON.Send(ws, req)
}

//HandleServer relays request r to websocket and recieve response and writes it to w.
func HandleServer(name string, w http.ResponseWriter, r *http.Request, doAccept func(*ResponseWriter) bool) {
	mutex.RLock()
	wsr := sockets[name]
	mutex.RUnlock()
	if wsr == nil {
		log.Println("not found", name)
		return
	}

	re := fromRequest(r, nil)
	wsr.msg <- re
	log.Println("sent request to websocket", re)

	var res ResponseWriter
	if err := websocket.JSON.Receive(wsr.ws, &res); err != nil {
		log.Println(err)
		wsr.stop <- struct{}{}
		return
	}
	log.Println("recv response from websocket")
	if doAccept != nil && !doAccept(&res) {
		log.Println("reponse is denied")
		return
	}
	if err := res.copyTo(w); err != nil {
		log.Println(err)
		return
	}
}

func setDeadlines(ws *websocket.Conn) {
	if err := ws.SetDeadline(time.Now().Add(100 * time.Hour)); err != nil {
		log.Fatal(err)
	}
	if err := ws.SetReadDeadline(time.Now().Add(100 * time.Hour)); err != nil {
		log.Fatal(err)
	}
	if err := ws.SetWriteDeadline(time.Now().Add(100 * time.Hour)); err != nil {
		log.Fatal(err)
	}
}

func close(err error, closed chan struct{}) {
	log.Println(err)
	if closed != nil {
		closed <- struct{}{}
	}
}

var clientWS *websocket.Conn

func readClient(serveHTTP http.HandlerFunc, closed chan struct{}, director func(*http.Request)) {
	for {
		var r request
		if err := websocket.JSON.Receive(clientWS, &r); err != nil {
			close(err, closed)
			return
		}
		log.Println("received req from websocket", r)
		if r.IsPing {
			log.Println("received ping")
			if err := sendPing(clientWS); err != nil {
				close(err, closed)
				return
			}
			continue
		}
		re, err := r.toRequest()
		if err != nil {
			log.Println(err)
			continue
		}
		if director != nil {
			director(re)
		}
		var w ResponseWriter
		serveHTTP(&w, re)
		if err := websocket.JSON.Send(clientWS, &w); err != nil {
			close(err, closed)
			return
		}
		log.Println("sent resp to websocket", re)
	}
}

//HandleClient connects to relayURL with websocket , reads requests and passes to
//serveMux, and write its response to websocket.
func HandleClient(relayURL, origin string, serveHTTP http.HandlerFunc, closed chan struct{}, director func(*http.Request)) error {
	if clientWS != nil {
		log.Println("closing openned websocket")
		if err := clientWS.Close(); err != nil {
			log.Println(err)
		}
	}
	var err error
	clientWS, err = websocket.Dial(relayURL, "", origin)
	if err != nil {
		log.Println(err)
		return err
	}
	setDeadlines(clientWS)
	go readClient(serveHTTP, closed, director)
	return nil
}
