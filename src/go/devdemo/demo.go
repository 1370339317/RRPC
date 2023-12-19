package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
)

// 序列化处理------------------------------------------------------------------------------------------------------------------------------
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
	// 创建一个长度为4的切片来读取头部
	header := make([]byte, 4)
	_, err := io.ReadFull(c.conn, header)
	if err != nil {
		return err
	}

	// 将头部转换为整数，这是消息的长度
	length := binary.BigEndian.Uint32(header)

	// 创建一个切片来读取消息
	data := make([]byte, length)
	_, err = io.ReadFull(c.conn, data)
	if err != nil {
		return err
	}

	// 使用json.Unmarshal函数将消息反序列化为v指向的值
	err = json.Unmarshal(data, v)
	if err != nil {
		return fmt.Errorf("TransparentCodec: failed to unmarshal data: %v", err)
	}

	return nil
}

// 发送接收代码------------------------------------------------------------------------------------------------------------------------------
type MyPack struct {
	Type  string
	Value string
}

type Client struct {
	codec       Codec
	sendCh      chan *MyPack
	recvCh      chan *MyPack
	serverReqCh chan *MyPack
	errCh       chan error
}

func Dial(address string) (*Client, error) {
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return nil, err
	}

	codec := NewTransparentCodec(conn)

	client := &Client{
		codec:  codec,
		sendCh: make(chan *MyPack),
		recvCh: make(chan *MyPack),
		errCh:  make(chan error),
	}

	go client.sendRequests()
	go client.receiveResponses()

	return client, nil
}

func (c *Client) sendRequests() {
	for req := range c.sendCh {
		err := c.codec.Encode(req)
		if err != nil {
			c.errCh <- err
			return
		}
	}
}

func (c *Client) receiveResponses() {
	for {
		res := &MyPack{}
		err := c.codec.Decode(res)
		if err != nil {
			c.errCh <- err
			return
		}
		switch res.Type {
		case "Response":

			c.recvCh <- res

		case "ServerRequest":

			c.serverReqCh <- res

		default:
			c.errCh <- fmt.Errorf("unknown message type: %s", res.Type)
			return
		}

	}
}

func (c *Client) Call(req *MyPack) (*MyPack, error) {
	c.sendCh <- req

	select {
	case res := <-c.recvCh:
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
		// handle the server request
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
