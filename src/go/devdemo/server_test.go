package main

import (
	"testing"
	"time"
)

func TestServer(t *testing.T) {

	server, err := NewServer("127.0.0.1:8080", func(c *Client) error {
		c.RegisterHandler("ToUpper", ToUpper)
		c.RegisterHandler("Add", Add)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	// The server is now running in the background.
	// We can do other stuff here, or just sleep.
	time.Sleep(1 * time.Second)
}
