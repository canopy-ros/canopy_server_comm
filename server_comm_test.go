package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
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

// TestServerCommHandler tests the communication
// server handler's response to a valid request.
func TestServerCommHandler(t *testing.T) {

	// initialize server
	db = "none"
	log.Println(db)
	r := &wsHandler{h: newHub()}
	srv := httptest.NewServer(r)
	defer srv.Close()
	/*
			srv := &http.Server{
				Addr:    ":8000",
				Handler: r,
			}
		    resp, err := http.Get(srv.URL)
	*/

	// create mock request
	url := fmt.Sprintf("%s/PsFXjmWpszr6acSKL/test_robot/state", srv.URL)
	req, err := http.NewRequest("GET", url, nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-Websocket-Version", "13")
	req.Header.Set("Sec-Websocket-Key", "7sCORs4/dvKiJRJb+vcZCA==")
	req.Header.Set("Accept-Encoding", "gzip")

	// send request and inspect response
	/*
		resp := httptest.NewRecorder()
		h.ServeHTTP(resp, req)
		if err != nil {
			t.Fatal(err)
		}
		log.Println("Status code:", resp.Code)
		log.Println("Response:", resp)
	*/
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	log.Println("Status code:", resp.StatusCode)
	log.Println("Response:", resp)
}
