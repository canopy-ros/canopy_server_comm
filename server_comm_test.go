package main

import (
	"fmt"
	"github.com/gorilla/websocket"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

// TestServerComm tests if communication server can
// run without errors.
func TestServerComm(t *testing.T) {
	r := &wsHandler{}
	srv := &http.Server{
		Addr:    ":8000",
		Handler: r,
	}

	ticker := time.NewTicker(time.Second)
	go func() {
		for t := range ticker.C {
			log.Println("Tick at", t)
		}
	}()

	go srv.ListenAndServe()
	log.Println("Server is running")
	time.Sleep(time.Second * 5)
	ticker.Stop()
	srv.Close()
	log.Println("Server has been closed")
}

// TestWsHandler tests that the communication server's
// websocket handler can receive messages successfully.
func TestWsHandler(t *testing.T) {

	// initialize server
	db = "none"
	r := &wsHandler{h: newHub()}
	srv := httptest.NewServer(r)

	// create websocket connection to server
	u, _ := url.Parse(fmt.Sprintf("%s/PsFXjmWpszr6acSKL/test_robot/description", srv.URL))
	u.Scheme = "ws"
	ws, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()

	// send test messages
	for i := 0; i < 5; i++ {
		log.Printf("Sending message %d", i)
		err = ws.WriteMessage(websocket.BinaryMessage, []byte("test"))
		if err != nil {
			t.Fatal(err)
		}
		_, _, err := ws.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
	}
	srv.Close()
}
