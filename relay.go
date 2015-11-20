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
	"io/ioutil"
	"log"
	"net/http"
	"net/url"

	"golang.org/x/net/websocket"
)

//Request is for relaying http.request , which doesn't include ones that cannot be converted.
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
	Close            bool
	Host             string
	Form             url.Values
	Trailer          http.Header
	RemoteAddr       string
	RequestURI       string
	Error            error
}

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

//Response is for relaying http.Response , which doesn't include ones that cannot be converted.
type response struct {
	Status           string // e.g. "200 OK"
	StatusCode       int    // e.g. 200
	Proto            string // e.g. "HTTP/1.0"
	ProtoMajor       int    // e.g. 1
	ProtoMinor       int    // e.g. 0
	Header           http.Header
	Body             []byte
	ContentLength    int64
	TransferEncoding []string
	Close            bool
	Trailer          http.Header
	Request          *request
	Error            error
}

func fromResponse(r *http.Response, err error) *response {
	re := &response{
		Status:           r.Status,
		StatusCode:       r.StatusCode,
		Proto:            r.Proto,
		ProtoMajor:       r.ProtoMajor,
		ProtoMinor:       r.ProtoMinor,
		Header:           r.Header,
		ContentLength:    r.ContentLength,
		TransferEncoding: r.TransferEncoding,
		Close:            r.Close,
		Trailer:          r.Trailer,
		Error:            err,
	}
	re.Body, err = ioutil.ReadAll(r.Body)
	err2 := r.Body.Close()
	if err != nil {
		log.Println(err)
		re.Error = err
		return re
	}
	if err2 != nil {
		log.Println(err2)
		re.Error = err
		return re
	}
	re.Request = fromRequest(r.Request, nil)
	return re
}

func (r *response) toResponse() (*http.Response, error) {
	re := &http.Response{}
	re.Status = r.Status
	re.StatusCode = r.StatusCode
	re.Proto = r.Proto
	re.ProtoMajor = r.ProtoMajor
	re.ProtoMinor = r.ProtoMinor
	re.Header = r.Header
	re.Body = ioutil.NopCloser(bytes.NewReader(r.Body))
	re.ContentLength = r.ContentLength
	re.TransferEncoding = r.TransferEncoding
	re.Close = r.Close
	re.Trailer = r.Trailer
	return re, r.Error
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

func (r *ResponseWriter) copyTo(w http.ResponseWriter) error {
	w.WriteHeader(r.StatusCode)
	if _, err := w.Write(r.Body); err != nil {
		return err
	}
	for k, vs := range r.Head {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	return nil
}

var sockets = make(map[string]*wsRelayServer)

type wsRelayServer struct {
	ws   *websocket.Conn
	stop chan struct{}
}

//ServeRelay starts to relay.
//It registers ws connection as name and wait for w.stop channel signal.
func ServeRelay(name string, ws *websocket.Conn, doAccept func(*http.Request) bool) {
	w := &wsRelayServer{
		ws:   ws,
		stop: make(chan struct{}),
	}
	sockets[name] = w
	log.Println("start serving relay")

	go func() {
		for {
			var req request
			if err := websocket.JSON.Receive(ws, &req); err != nil {
				log.Println(err)
				w.stop <- struct{}{}
				return
			}
			r, err := req.toRequest()
			if err != nil {
				log.Println(err)
				w.stop <- struct{}{}
				return
			}
			if doAccept != nil && !doAccept(r) {
				log.Println("request is denied")
				w.stop <- struct{}{}
				return
			}
			client := http.Client{}
			resp, err := client.Do(r)
			if err != nil {
				log.Println(err)
			}
			re := fromResponse(resp, err)
			if err := websocket.JSON.Send(ws, re); err != nil {
				log.Println(err)
			}
		}
	}()

	<-w.stop
	log.Println("relay exited")
	if err := ws.Close(); err != nil {
		log.Println(err)
	}

}

//StopServeRelay stops relaying associated with name.
func StopServeRelay(name string) {
	if w, exist := sockets[name]; exist {
		w.stop <- struct{}{}
	}
}

//HandleRelayServer relays request r to websocket and recieve response and writes it to w.
func HandleRelayServer(name string, w http.ResponseWriter, r *http.Request, doAccept func(*ResponseWriter) bool) {
	wsr := sockets[name]
	if wsr == nil {
		log.Println("not found", name)
		return
	}
	ws := wsr.ws

	re := fromRequest(r, nil)
	if err := websocket.JSON.Send(ws, re); err != nil {
		log.Println(err)
		return
	}
	log.Println("sent request to websocket", re)

	var res ResponseWriter
	if err := websocket.JSON.Receive(ws, &res); err != nil {
		log.Println(err)
		return
	}
	if doAccept != nil && doAccept(&res) {
		log.Println("reponse is denied")
		wsr.stop <- struct{}{}
		return
	}
	if err := res.copyTo(w); err != nil {
		log.Println(err)
		return
	}
}

//HandleRelayClient connects to relayURL with websocket , reads requests and passes to
//serveMux, and write its response to websocket.
func HandleRelayClient(relayURL, origin string, serveMux *http.ServeMux, modify func(*http.Request)) (*websocket.Conn, error) {
	ws, err := websocket.Dial(relayURL, "", origin)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	go func() {
		for {
			var r request
			if err := websocket.JSON.Receive(ws, &r); err != nil {
				log.Fatal(err)
				continue
			}
			log.Println("received req from websocket:", r)
			re, err := r.toRequest()
			if err != nil {
				log.Println(err)
				continue
			}
			if modify != nil {
				modify(re)
			}
			var w ResponseWriter
			serveMux.ServeHTTP(&w, re)
			if err := websocket.JSON.Send(ws, w); err != nil {
				log.Println(err)
			}
			log.Println("sent resp to websocket")
		}
	}()
	return ws, nil
}

//SendHTTPRequest is for client to request http via websocket.
func SendHTTPRequest(ws *websocket.Conn, rq *http.Request) (*http.Response, error) {
	req := fromRequest(rq, nil)
	if err := websocket.JSON.Send(ws, &req); err != nil {
		log.Println(err)
		return nil, err
	}
	var re response
	if err := websocket.JSON.Receive(ws, &re); err != nil {
		log.Println(err)
		return nil, err
	}
	return re.toResponse()
}
