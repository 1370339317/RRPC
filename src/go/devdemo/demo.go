package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync/atomic"
)

type Codec interface {
	Encode(interface{}) error
	Decode(interface{}) error
}

type TransparentCodec struct {
	conn net.Conn
}

func NewTransparentCodec(conn net.Conn) *TransparentCodec {
	return &TransparentCodec{conn: conn}
}

func (c *TransparentCodec) Encode(v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("TransparentCodec: unsupported data type %T", v)
	}
	_, err = c.conn.Write(data)
	return err
}

func (c *TransparentCodec) Decode(v interface{}) error {
	header := make([]byte, 4)
	_, err := io.ReadFull(c.conn, header)
	if err != nil {
		return err
	}

	length := binary.BigEndian.Uint32(header)

	data := make([]byte, length)
	_, err = io.ReadFull(c.conn, data)
	if err != nil {
		return err
	}

	err = json.Unmarshal(data, v)
	if err != nil {
		return fmt.Errorf("TransparentCodec: failed to unmarshal data: %v", err)
	}

	return nil
}

type MyPack struct {
	ID    uint64
	Type  string
	Value string
}

type Client struct {
	codec       Codec
	sendCh      chan *MyPack
	recvCh      chan *MyPack
	serverReqCh chan *MyPack
	errCh       chan error
	seq         uint64
	resChs      map[uint64]chan *MyPack
}

func Dial(address string) (*Client, error) {
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return nil, err
	}

	codec := NewTransparentCodec(conn)

	client := &Client{
		codec:       codec,
		sendCh:      make(chan *MyPack),
		recvCh:      make(chan *MyPack),
		serverReqCh: make(chan *MyPack),
		errCh:       make(chan error),
		resChs:      make(map[uint64]chan *MyPack),
	}

	go client.sendRequests()
	go client.receiveResponses()

	return client, nil
}

func (c *Client) sendRequests() {
	defer close(c.sendCh)
	for req := range c.sendCh {
		err := c.codec.Encode(req)
		if err != nil {
			c.errCh <- err
			return
		}
	}
}

func (c *Client) receiveResponses() {
	defer func() {
		for _, ch := range c.resChs {
			close(ch)
		}
		close(c.recvCh)
		close(c.serverReqCh)
	}()
	for {
		res := &MyPack{}
		err := c.codec.Decode(res)
		if err != nil {
			c.errCh <- err
			return
		}
		switch res.Type {
		case "Response":
			if ch, ok := c.resChs[res.ID]; ok {
				ch <- res
				delete(c.resChs, res.ID)
			} else {
				c.errCh <- fmt.Errorf("no channel found for response ID %d", res.ID)
			}
		case "ServerRequest":
			c.serverReqCh <- res
		default:
			c.errCh <- fmt.Errorf("unknown message type: %s", res.Type)
			return
		}
	}
}

func (c *Client) Call(req *MyPack) (*MyPack, error) {
	req.ID = atomic.AddUint64(&c.seq, 1)
	resCh := make(chan *MyPack)
	c.resChs[req.ID] = resCh
	c.sendCh <- req

	select {
	case res := <-resCh:
		return res, nil
	case err := <-c.errCh:
		return nil, err
	}
}

func (c *Client) HandleServerRequest(handler func(*MyPack)) {
	for req := range c.serverReqCh {
		handler(req)
	}
}

func main() {
	client, err := Dial("127.0.0.1:6688")
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	go client.HandleServerRequest(func(req *MyPack) {
		println("这是一个来自服务端的rpc调用", req.Value)
	})

	req := &MyPack{
		Type:  "Request",
		Value: "hello world",
	}

	res, err := client.Call(req)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Println("Server reply:", res.Value)
}
