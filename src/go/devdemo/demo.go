package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"
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
	Args       string // 请求参数
	Result     string // 响应结果
}

const (
	RequestType       = "Request"
	ResponseType      = "Response"
	ServerRequestType = "ServerRequest"
)

type InvokeResult struct {
	Result string
	Err    error
}

type HandlerFunc interface{}

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

func setArgValue(v reflect.Value, result interface{}) error {
	if v.Kind() == reflect.Ptr {
		switch v.Elem().Kind() {
		case reflect.Int:
			v.Elem().SetInt(int64(result.(float64)))
		case reflect.Float64:
			v.Elem().SetFloat(result.(float64))
		case reflect.String:
			v.Elem().SetString(result.(string))
		case reflect.Bool:
			v.Elem().SetBool(result.(bool))
		// ... 其他类型
		default:
			return fmt.Errorf("unsupported type: %s", v.Elem().Type())
		}
	}
	return nil
}

func (c *Client) Invoke(method string, args ...interface{}) *InvokeResult {
	// 将参数序列化为JSON字符串
	value, err := json.Marshal(args)
	if err != nil {
		return &InvokeResult{Err: fmt.Errorf("failed to marshal args: %v", err)}
	}

	// 创建请求对象
	req := &MyPack{
		Type:       RequestType,
		MethodName: method,
		Args:       string(value),
	}

	// 发送请求并获取响应
	res, err := c.Call(req)
	if err != nil {
		return &InvokeResult{Err: err}
	}

	// 反序列化参数的引用
	var result []interface{}
	err = json.Unmarshal([]byte(res.Args), &result)
	if err != nil {
		return &InvokeResult{Err: fmt.Errorf("failed to unmarshal result: %v", err)}
	}

	// 创建结果对象
	invokeResult := &InvokeResult{
		Result: res.Result,
		Err:    nil,
	}

	// 更新指针参数的值
	for i, arg := range args {
		err := setArgValue(reflect.ValueOf(arg), result[i])
		if err != nil {
			invokeResult.Err = err
			break
		}
	}

	return invokeResult
}

func (c *Client) Reply(req *MyPack, value string) error {
	res := &MyPack{
		ID:     req.ID, // 使用原来请求的ID
		Type:   ResponseType,
		Result: value,
	}
	c.sendCh <- res
	return nil
}

func (c *Client) HandleServerRequest() {
	c.pool.Start()
	go func() {
		for pack := range c.serverReqCh {
			handler, ok := c.handlerMap[pack.MethodName]
			if !ok {
				fmt.Printf("No handler found for %s\n", pack.Type)
				continue
			}

			c.pool.Submit(func() {
				defer func() {
					if r := recover(); r != nil {
						fmt.Printf("Handler panic: %v\n", r)
					}
				}()
				// 解析参数数组
				var args []interface{}
				err := json.Unmarshal([]byte(pack.Args), &args)
				if err != nil {
					fmt.Printf("Failed to unmarshal args: %v\n", err)
					return
				}

				// 反射创建函数参数
				funcValue := reflect.ValueOf(handler)
				funcType := funcValue.Type()
				numIn := funcType.NumIn()

				// 检查参数数量
				if len(args) != numIn {
					fmt.Printf("Wrong number of arguments for %s: expected %d, got %d\n", pack.MethodName, numIn, len(args))
					return
				}

				in := make([]reflect.Value, numIn)
				for i := range in {
					in[i] = reflect.New(funcType.In(i)).Elem()
				}

				// 将解析的参数值赋给函数参数
				for i, arg := range args {
					argValue := reflect.ValueOf(arg)
					if argValue.Type() != in[i].Type() {
						if argValue.Kind() == reflect.Float64 && in[i].Kind() == reflect.Int {
							// 将 float64 类型转换为 int 类型
							in[i].SetInt(int64(argValue.Float()))
						} else {
							fmt.Printf("Wrong type of argument for %s: expected %s, got %s\n", pack.MethodName, in[i].Type(), argValue.Type())
							return
						}
					} else {
						in[i].Set(argValue)
					}
				}

				// 反射调用处理函数
				out := funcValue.Call(in)

				// 获取处理函数的返回值
				var result string
				var returnErr error
				if len(out) > 0 {
					if len(out) == 2 {
						// 如果有两个返回值，那么第二个应该是 error 类型
						if err, ok := out[1].Interface().(error); ok {
							returnErr = err
						}
					}
					result = fmt.Sprintf("%v", out[0].Interface())
				}

				if returnErr != nil {
					fmt.Printf("Handler error: %s\n", returnErr)
					return
				}

				err = c.Reply(pack, result)
				if err != nil {
					fmt.Printf("Reply error: %s\n", err)
				}
			})
		}
	}()
}

func (c *Client) RegisterHandler(name string, handler HandlerFunc) {
	v := reflect.ValueOf(handler)
	if v.Kind() != reflect.Func {
		fmt.Printf("Handler is not a function: %s\n", name)
		return
	}
	c.handlerMap[name] = handler
}

func (c *Client) GenerateDocs() string {
	var docs []map[string]interface{}

	for name, handler := range c.handlerMap {
		handlerType := reflect.TypeOf(handler)
		var params []string
		var returns []string

		for i := 0; i < handlerType.NumIn(); i++ {
			params = append(params, handlerType.In(i).String())
		}

		for i := 0; i < handlerType.NumOut(); i++ {
			returns = append(returns, handlerType.Out(i).String())
		}

		doc := map[string]interface{}{
			"function":   name,
			"parameters": params,
			"returns":    returns,
		}

		docs = append(docs, doc)
	}

	jsonDocs, err := json.MarshalIndent(docs, "", "  ")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(jsonDocs))
	return string(jsonDocs)
}

func GenerateWrappers(doc string) {
	var funcs []struct {
		Function   string   `json:"function"`
		Parameters []string `json:"parameters"`
		Returns    []string `json:"returns"`
	}

	err := json.Unmarshal([]byte(doc), &funcs)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("type MyServiceClient struct {")
	fmt.Println("\tclient *Client")
	fmt.Println("}")

	for _, f := range funcs {
		fmt.Printf("\nfunc (s *MyServiceClient) %s(", f.Function)
		for i, p := range f.Parameters {
			if i > 0 {
				fmt.Print(", ")
			}
			fmt.Printf("arg%d %s", i, p)
		}
		fmt.Print(") (")
		for i, r := range f.Returns {
			if i > 0 {
				fmt.Print(", ")
			}
			if r == "error" {
				fmt.Print("error")
			} else {
				fmt.Printf("ret%d %s", i, r)
			}
		}
		fmt.Println(") {")
		fmt.Printf("\tresult := s.client.Invoke(\"%s\"", f.Function)
		for i := range f.Parameters {
			fmt.Printf(", arg%d", i)
		}
		fmt.Println(")")
		fmt.Println("\tif result.Err != nil {")
		fmt.Print("\t\treturn ")
		for i, r := range f.Returns {
			if i > 0 {
				fmt.Print(", ")
			}
			if r == "error" {
				fmt.Print("result.Err")
			} else {
				fmt.Print("nil")
			}
		}
		fmt.Println("\n\t}")
		fmt.Print("\treturn ")
		for i, r := range f.Returns {
			if i > 0 {
				fmt.Print(", ")
			}
			if r == "error" {
				fmt.Print("nil")
			} else {
				fmt.Printf("strconv.Atoi(result.Result)")
			}
		}
		fmt.Println("\n}")
	}
}

func main() {
	MakeWrapper("HHServer", "rpcprovider.go", "rpcwrapper.go")
	client, err := Dial("127.0.0.1:6688")
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	client.RegisterHandler("ToUpper",
		func(s string, a1, a2, a3 int) string {
			// 这是你的函数，你可以在这里处理服务器的RPC请求
			fmt.Println("收到来自服务器的RPC请求:", s)
			return strings.ToUpper(s)
		})

	client.RegisterHandler("Add",
		func(a, b int) int {
			// 这是另一个函数，你可以在这里处理服务器的RPC请求
			fmt.Println("收到来自服务器的RPC请求:", a, b)
			return a + b
		})

	client.HandleServerRequest() // 启动处理服务端请求的goroutine

	GenerateWrappers(client.GenerateDocs())

	arg1 := 1 //测试引用参返回数据
	result := client.Invoke("ToUpper", "hello", &arg1, 2, 3)
	if result.Err != nil {
		fmt.Println("Invoke error:", result.Err)
	}
	fmt.Println("Server reply:", result.Result)

	time.Sleep(666 * time.Second)
}
