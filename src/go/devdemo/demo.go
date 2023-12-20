package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
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

type WorkerPool struct {
	workers int
	jobs    chan func()
}

func NewWorkerPool(workers int, handler func(*MyPack)) *WorkerPool {
	return &WorkerPool{
		workers: workers,
		jobs:    make(chan func()),
	}
}

func (p *WorkerPool) Start() {
	for i := 0; i < p.workers; i++ {
		go func() {
			for job := range p.jobs {
				job() // 调用函数
			}
		}()
	}
}

func (p *WorkerPool) Submit(job func()) {
	p.jobs <- job
}

type MyPack struct {
	ID         uint64
	Type       string // 报文类型：Request, Response, ServerRequest
	MethodName string // 函数名
	Value      string
}

const (
	RequestType       = "Request"
	ResponseType      = "Response"
	ServerRequestType = "ServerRequest"
)

type HandlerFunc func(*MyPack) (string, error)

type Client struct {
	codec       Codec
	sendCh      chan *MyPack
	recvCh      chan *MyPack
	serverReqCh chan *MyPack
	errCh       chan error
	seq         uint64
	resChs      sync.Map
	handlerMap  map[string]HandlerFunc
	pool        *WorkerPool
}

func Dial(address string) (*Client, error) {
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return nil, err
	}

	codec := NewTransparentCodec(conn)

	pool := NewWorkerPool(10, nil) // 创建一个没有处理函数的WorkerPool

	client := &Client{
		codec:       codec,
		sendCh:      make(chan *MyPack),
		recvCh:      make(chan *MyPack),
		serverReqCh: make(chan *MyPack),
		errCh:       make(chan error),
		resChs:      sync.Map{},
		handlerMap:  make(map[string]HandlerFunc),
		pool:        pool,
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
		c.resChs.Range(func(key, value interface{}) bool {
			close(value.(chan *MyPack))
			return true
		})
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
			value, ok := c.resChs.Load(res.ID)
			if ok {
				ch := value.(chan *MyPack)
				ch <- res
				c.resChs.Delete(res.ID)
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
	req.Type = RequestType // Call方法
	req.ID = atomic.AddUint64(&c.seq, 1)
	resCh := make(chan *MyPack)
	c.resChs.Store(req.ID, resCh)
	c.sendCh <- req

	select {
	case res := <-resCh:
		return res, nil
	case err := <-c.errCh:
		return nil, err
	}
}
func (c *Client) Reply(req *MyPack, value string) error {
	res := &MyPack{
		ID:    req.ID, // 使用原来请求的ID
		Type:  ResponseType,
		Value: value,
	}
	c.sendCh <- res
	return nil
}

func (c *Client) HandleServerRequest() {
	c.pool.Start() // 启动工作池
	go func() {
		for req := range c.serverReqCh {
			handler, ok := c.handlerMap[req.MethodName]

			if ok {
				c.pool.Submit(func() {
					value, err := handler(req)
					if err != nil {
						fmt.Printf("Handler error: %s\n", err)
						return
					}
					err = c.Reply(req, value)
					if err != nil {
						fmt.Printf("Reply error: %s\n", err)
					}
				})
			} else {
				fmt.Printf("No handler found for %s\n", req.Type)
			}
		}
	}()
}

func (c *Client) RegisterHandler(name string, handler HandlerFunc) {
	c.handlerMap[name] = handler
}

func main() {
	client, err := Dial("127.0.0.1:6688")
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	client.RegisterHandler("ToUpper", func(req *MyPack) (string, error) {
		// 这是你的函数，你可以在这里处理服务器的RPC请求
		fmt.Println("收到来自服务器的RPC请求:", req.Value)

		// 假设服务器请求的是一个字符串转大写的操作
		value := strings.ToUpper(req.Value)

		// 返回结果和错误
		return value, nil
	})

	client.RegisterHandler("ToLower", func(req *MyPack) (string, error) {
		// 这是另一个函数，你可以在这里处理服务器的RPC请求
		return "", nil
	})
	client.HandleServerRequest() // 启动处理服务端请求的goroutine
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
