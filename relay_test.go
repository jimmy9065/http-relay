package relay

import (
	"io/ioutil"
	"log"
	"net/http"
	"testing"
	"time"

	"golang.org/x/net/websocket"
)

func TestRelay(t *testing.T) {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	go func() {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			HandleRelayServer("test", "http://localhost:1234/hello", w, r)
		})
		http.Handle("/ws", websocket.Handler(func(ws *websocket.Conn) {
			ServeRelay("test", ws)
		}))

		if err := http.ListenAndServe(":1234", nil); err != nil {
			log.Fatal("ListenAndServe:", err)
		}
	}()

	time.Sleep(time.Second)

	go func() {
		http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
			if _, err := w.Write([]byte("hello world!")); err != nil {
				t.Fatal(err)
			}
		})
		origin := "http://localhost/"
		url := "ws://localhost:1234/ws"
		err := HandleRelayClient(url, origin, http.DefaultServeMux)
		if err != nil {
			log.Fatal(err)
		}
	}()

	time.Sleep(time.Second)

	log.Println("requesting")
	res, err := http.Get("http://localhost:1234/")
	if err != nil {
		t.Fatal(err)
	}
	body, err := ioutil.ReadAll(res.Body)
	err2 := res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if err2 != nil {
		t.Fatal(err2)
	}

	log.Println("res from http", string(body), res)
	if string(body) != "hello world!" {
		t.Fatal("response unmatched")
	}
}
