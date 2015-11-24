package relay

import (
	"io/ioutil"
	"log"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestRelay(t *testing.T) {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	//relay server
	go func() {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			HandleServer("test", w, r, func(r *ResponseWriter) bool {
				return true
			})
		})
		http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
			ug := websocket.Upgrader{}
			ws, err := ug.Upgrade(w, r, nil)
			if err != nil {
				log.Print("upgrade:", err)
				return
			}
			StartServe("test", ws)
		})

		if err := http.ListenAndServe(":1234", nil); err != nil {
			log.Fatal("ListenAndServe:", err)
		}
	}()

	time.Sleep(time.Second)

	//relay client
	go func() {
		http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
			if _, err := w.Write([]byte("hello world!")); err != nil {
				t.Fatal(err)
			}
		})
		url := "ws://localhost:1234/ws"
		err := HandleClient(url, http.DefaultServeMux.ServeHTTP, nil, func(r *http.Request) {
			r.URL.Path = "/hello"
		})
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
