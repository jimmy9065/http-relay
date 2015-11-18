[![Build Status](https://travis-ci.org/shingetsu-gou/http-relay.svg?branch=master)](https://travis-ci.org/shingetsu-gou/http-relay)
[![GoDoc](https://godoc.org/github.com/shingetsu-gou/http-relay?status.svg)](https://godoc.org/github.com/shingetsu-gou/http-relay)
[![GitHub license](https://img.shields.io/badge/license-MIT-blue.svg)](https://raw.githubusercontent.com/shingetsu-gou/http-relay/master/LICENSE)


# http-relay 

## Overview

http-relay is a module for relaying http by websocket in golang.

When one node B is behind NAT and needs to be connected from node C(needs NAT traversal),
C connects to relay node A with websocket and C relays data to B by this module.


## Platform
  * MacOS darwin/Plan9 on i386
  * Windows/OpenBSD on i386/amd64
  * Linux/NetBSD/FreeBSD on i386/amd64/arm
  * Solaris on amd64

## Example

Suppose C wants to communicate with B by relaying A, 
i.e. C <-> A <-> B:

```go

//relay server A
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		relay.HandleRelayServer("test", "http://localhost:1234/hello", w, r)
	})
	http.Handle("/ws", websocket.Handler(func(ws *websocket.Conn) {
		relay.ServeRelay("test", ws)
	}))
	http.ListenAndServe(":1234", nil)


//relay client B
	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello world!"))
	})
	origin := "http://localhost/"
	url := "ws://localhost:1234/ws"
	relay.HandleRelayClient(url, origin, http.DefaultServeMux)

//node that want to connect relay client C
	res, _:= http.Get("http://localhost:1234/")
	body, _:= ioutil.ReadAll(res.Body)
    res.Body.Close()
	//body must be "hello world!"

```

## Requirements

* git
* go 1.4+

are required to compile.

## Compile

    $ mkdir gou
    $ cd gou
    $ mkdir src
    $ mkdir bin
    $ mkdir pkg
    $ exoprt GOPATH=`pwd`
    $ go get github.com/shingetsu-gou/http-relay
	
## License

MIT License

# Contribution

Improvements to the codebase and pull requests are encouraged.
