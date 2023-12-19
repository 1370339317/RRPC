package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
)

type Request struct {
	Op    string
	Value string
}

type Response struct {
	Value string
}

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

func sendRequests(codec Codec, sendCh chan *Request, errCh chan error) {
	for req := range sendCh {
		err := codec.Encode(req)
		if err != nil {
			errCh <- err
			return
		}
	}
}

func receiveResponses(codec Codec, recvCh chan *Response, errCh chan error) {
	for {
		res := &Response{}
		err := codec.Decode(res)
		if err != nil {
			errCh <- err
			return
		}
		recvCh <- res
	}
}
func main() {
	conn, _ := net.Dial("tcp", "127.0.0.1:6688")
	defer conn.Close()

	codec := NewTransparentCodec(conn)

	sendCh := make(chan *Request)
	recvCh := make(chan *Response)
	errCh := make(chan error)

	go sendRequests(codec, sendCh, errCh)
	go receiveResponses(codec, recvCh, errCh)

	for {
		req := &Request{
			Op:    "upper",
			Value: "hello world",
		}

		sendCh <- req

		select {
		case res := <-recvCh:
			fmt.Println("Server reply:", res.Value)
		case err := <-errCh:
			fmt.Println("Error:", err)
			return
		}
	}
}
