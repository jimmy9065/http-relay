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
type Request struct {
	// Method specifies the HTTP method (GET, POST, PUT, etc.).
	// For client requests an empty string means GET.
	Method string

	// URL specifies either the URI being requested (for server
	// requests) or the URL to access (for client requests).
	//
	// For server requests the URL is parsed from the URI
	// supplied on the Request-Line as stored in RequestURI.  For
	// most requests, fields other than Path and RawQuery will be
	// empty. (See RFC 2616, Section 5.1.2)
	//
	// For client requests, the URL's Host specifies the server to
	// connect to, while the Request's Host field optionally
	// specifies the Host header value to send in the HTTP
	// request.
	URL *url.URL

	// The protocol version for incoming requests.
	// Client requests always use HTTP/1.1.
	Proto      string // "HTTP/1.0"
	ProtoMajor int    // 1
	ProtoMinor int    // 0

	// A header maps request lines to their values.
	// If the header says
	//
	//	accept-encoding: gzip, deflate
	//	Accept-Language: en-us
	//	Connection: keep-alive
	//
	// then
	//
	//	Header = map[string][]string{
	//		"Accept-Encoding": {"gzip, deflate"},
	//		"Accept-Language": {"en-us"},
	//		"Connection": {"keep-alive"},
	//	}
	//
	// HTTP defines that header names are case-insensitive.
	// The request parser implements this by canonicalizing the
	// name, making the first character and any characters
	// following a hyphen uppercase and the rest lowercase.
	//
	// For client requests certain headers are automatically
	// added and may override values in Header.
	//
	// See the documentation for the Request.Write method.
	Header http.Header

	// Body is the request's body.
	//
	// For client requests a nil body means the request has no
	// body, such as a GET request. The HTTP Client's Transport
	// is responsible for calling the Close method.
	//
	// For server requests the Request Body is always non-nil
	// but will return EOF immediately when no body is present.
	// The Server will close the request body. The ServeHTTP
	// Handler does not need to.
	Body []byte

	// ContentLength records the length of the associated content.
	// The value -1 indicates that the length is unknown.
	// Values >= 0 indicate that the given number of bytes may
	// be read from Body.
	// For client requests, a value of 0 means unknown if Body is not nil.
	ContentLength int64

	// TransferEncoding lists the transfer encodings from outermost to
	// innermost. An empty list denotes the "identity" encoding.
	// TransferEncoding can usually be ignored; chunked encoding is
	// automatically added and removed as necessary when sending and
	// receiving requests.
	TransferEncoding []string

	// Close indicates whether to close the connection after
	// replying to this request (for servers) or after sending
	// the request (for clients).
	Close bool

	// For server requests Host specifies the host on which the
	// URL is sought. Per RFC 2616, this is either the value of
	// the "Host" header or the host name given in the URL itself.
	// It may be of the form "host:port".
	//
	// For client requests Host optionally overrides the Host
	// header to send. If empty, the Request.Write method uses
	// the value of URL.Host.
	Host string

	// Form contains the parsed form data, including both the URL
	// field's query parameters and the POST or PUT form data.
	// This field is only available after ParseForm is called.
	// The HTTP client ignores Form and uses Body instead.
	Form url.Values

	// Trailer specifies additional headers that are sent after the request
	// body.
	//
	// For server requests the Trailer map initially contains only the
	// trailer keys, with nil values. (The client declares which trailers it
	// will later send.)  While the handler is reading from Body, it must
	// not reference Trailer. After reading from Body returns EOF, Trailer
	// can be read again and will contain non-nil values, if they were sent
	// by the client.
	//
	// For client requests Trailer must be initialized to a map containing
	// the trailer keys to later send. The values may be nil or their final
	// values. The ContentLength must be 0 or -1, to send a chunked request.
	// After the HTTP request is sent the map values can be updated while
	// the request body is read. Once the body returns EOF, the caller must
	// not mutate Trailer.
	//
	// Few HTTP clients, servers, or proxies support HTTP trailers.
	Trailer http.Header

	// RemoteAddr allows HTTP servers and other software to record
	// the network address that sent the request, usually for
	// logging. This field is not filled in by ReadRequest and
	// has no defined format. The HTTP server in this package
	// sets RemoteAddr to an "IP:port" address before invoking a
	// handler.
	// This field is ignored by the HTTP client.
	RemoteAddr string

	// RequestURI is the unmodified Request-URI of the
	// Request-Line (RFC 2616, Section 5.1) as sent by the client
	// to a server. Usually the URL field should be used instead.
	// It is an error to set this field in an HTTP client request.
	RequestURI string
}

//Response is simple struct for http.ResponseWriter.
type Response struct {
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
func (r *Response) Header() http.Header {
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
func (r *Response) Write(d []byte) (int, error) {
	r.Body = append(r.Body, d...)
	return len(d), nil
}

// WriteHeader sends an HTTP response header with status code.
// If WriteHeader is not called explicitly, the first call to Write
// will trigger an implicit WriteHeader(http.StatusOK).
// Thus explicit calls to WriteHeader are mainly used to
// send error codes.
func (r *Response) WriteHeader(s int) {
	r.StatusCode = s
}

var sockets = make(map[string]*wsRelayServer)

type wsRelayServer struct {
	ws   *websocket.Conn
	stop chan struct{}
}

//ServeRelay starts to relay.
//It registers ws connection as name and wait for w.stop channel signal.
func ServeRelay(name string, ws *websocket.Conn) {
	w := &wsRelayServer{
		ws:   ws,
		stop: make(chan struct{}),
	}
	sockets[name] = w
	log.Println("start serving relay")
	select {
	case <-w.stop:
		log.Println("relay existed")
		return
	}
}

//StopServeRelay stops relaying associated with name.
func StopServeRelay(name string) {
	if w, exist := sockets[name]; exist {
		w.stop <- struct{}{}
	}
}

//HandleRelayServer relays request r to websocket and recieve response and writes it to w.
func HandleRelayServer(name, urlForClient string, w http.ResponseWriter, r *http.Request) {
	var err error
	wsr := sockets[name]
	if wsr == nil {
		log.Println("not found", name)
		return
	}
	ws := wsr.ws
	url, err := url.Parse(urlForClient)
	if err != nil {
		log.Println(err)
		return
	}

	re := Request{
		Method:           r.Method,
		URL:              url,
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
	}
	re.Body, err = ioutil.ReadAll(r.Body)
	err2 := r.Body.Close()
	if err != nil {
		log.Println(err)
		return
	}
	if err2 != nil {
		log.Println(err2)
		return
	}
	if err := websocket.JSON.Send(ws, re); err != nil {
		log.Println(err)
		return
	}
	log.Println("send request to websocket", re)

	var res Response
	if err := websocket.JSON.Receive(ws, &res); err != nil {
		log.Println(err)
		return
	}
	w.WriteHeader(res.StatusCode)
	if _, err := w.Write(res.Body); err != nil {
		log.Println(err)
		return
	}
	for k, vs := range res.Head {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
}

//HandleRelayClient connects to relayURL with websocket , reads requests and passes to
//serveMux, and write its response to websocket.
func HandleRelayClient(relayURL, origin string, serveMux *http.ServeMux) error {
	ws, err := websocket.Dial(relayURL, "", origin)
	if err != nil {
		log.Println(err)
		return err
	}
	for {
		var r Request
		if err := websocket.JSON.Receive(ws, &r); err != nil {
			log.Println(err)
			continue
		}
		log.Println("received req from websocket:", r)
		b := bytes.NewReader(r.Body)
		re, err := http.NewRequest(r.Method, r.URL.String(), b)
		if err != nil {
			log.Println(err)
			continue
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
		var w Response
		serveMux.ServeHTTP(&w, re)
		if err := websocket.JSON.Send(ws, w); err != nil {
			log.Println(err)
		}
		log.Println("send resp to websocket")
	}
}
