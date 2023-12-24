package main

import (
	flexpacketprotocol "devdemo/protocol"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"reflect"
	"sync"
	"sync/atomic"
	"time"
)

type Codec interface {
	Encode(interface{}) ([]byte, error)
	Decode([]byte, interface{}) error
}

type TransparentCodec struct{}

func NewTransparentCodec() *TransparentCodec {
	return &TransparentCodec{}
}

// Encode 方法返回序列化后的数据，而不是直接写入 conn
func (c *TransparentCodec) Encode(v interface{}) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("TransparentCodec: unsupported data type %T", v)
	}
	return data, nil
}

// Decode 方法从数据中反序列化，而不是直接从 conn 读取
func (c *TransparentCodec) Decode(data []byte, v interface{}) error {
	err := json.Unmarshal(data, v)
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
	Args       []byte // 请求参数
	Result     []byte // 响应结果
	Error      string // 错误信息
}

const (
	RequestType  = "Request"
	ResponseType = "Response"
)

type InvokeResult struct {
	Result []byte
	Err    error
}

type HandlerFunc interface{}

type Client struct {
	conn        net.Conn
	codec       Codec
	sendCh      chan *MyPack
	recvCh      chan *MyPack
	serverReqCh chan *MyPack
	errCh       chan error
	seq         uint64
	resChs      sync.Map
	handlerMap  map[string]HandlerFunc
	pool        *WorkerPool

	frameReader io.Reader
	frameWriter io.Writer
}

func (c *Client) writeToConn(data []byte) error {
	_, err := c.frameWriter.Write(data)
	return err
}

func (c *Client) readFromConn() ([]byte, error) {
	size := 1024 // 初始缓冲区大小
	for {
		data := make([]byte, size)
		n, err := c.frameReader.Read(data)
		if err != nil {
			if err.Error() == "buffer too small" {
				size *= 2 // 如果缓冲区太小，就增大它的大小
				continue
			}
			return nil, err
		}
		return data[:n], nil
	}
}

func (c *Client) send(v *MyPack) error {
	data, err := c.codec.Encode(v)
	if err != nil {
		return err
	}

	return c.writeToConn(data)
}

func (c *Client) receive(v *MyPack) error {
	data, err := c.readFromConn()
	if err != nil {
		return err
	}
	return c.codec.Decode(data, v)
}

func Dial(address string) (*Client, error) {
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return nil, err
	}

	codec := NewTransparentCodec()

	pool := NewWorkerPool(10, nil) // 创建一个没有处理函数的WorkerPool
	//报文协议
	protocol := flexpacketprotocol.New(conn, []byte("aacc"), []byte("eezz"))
	client := &Client{
		conn:        conn,
		codec:       codec,
		sendCh:      make(chan *MyPack),
		recvCh:      make(chan *MyPack),
		serverReqCh: make(chan *MyPack),
		errCh:       make(chan error),
		resChs:      sync.Map{},
		handlerMap:  make(map[string]HandlerFunc),
		pool:        pool,
		frameReader: protocol,
		frameWriter: protocol,
	}

	go client.sendRequests()
	go client.receiveResponses()

	return client, nil
}

type ClientHandler func(*Client) error
type Server struct {
	listener    net.Listener
	clients     []*Client
	onNewClient ClientHandler
}

func NewServer(address string, onNewClient func(*Client) error) (*Server, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}

	server := &Server{
		listener:    listener,
		clients:     make([]*Client, 0),
		onNewClient: onNewClient,
	}

	go server.acceptConnections()

	return server, nil
}
func (s *Server) Close() error {
	return s.listener.Close()
}
func (s *Server) NewClient(conn net.Conn) *Client {
	codec := NewTransparentCodec()
	pool := NewWorkerPool(10, nil)
	protocol := flexpacketprotocol.New(conn, []byte("aacc"), []byte("eezz"))
	client := &Client{
		conn:        conn,
		codec:       codec,
		sendCh:      make(chan *MyPack),
		recvCh:      make(chan *MyPack),
		serverReqCh: make(chan *MyPack),
		errCh:       make(chan error),
		resChs:      sync.Map{},
		handlerMap:  make(map[string]HandlerFunc),
		pool:        pool,
		frameReader: protocol,
		frameWriter: protocol,
	}

	//必须保证HandleServerRequest早于receiveResponses否则可能发生死锁
	client.HandleServerRequest()
	go client.sendRequests()
	go client.receiveResponses()

	return client
}
func (s *Server) acceptConnections() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			log.Println("Accept error:", err)
			continue
		}

		client := s.NewClient(conn)
		s.clients = append(s.clients, client)
		s.handleNewClient(client)
	}
}

func (s *Server) handleNewClient(client *Client) {

	// 在这里处理新的连接
	err := s.onNewClient(client) // 增加这一行
	if err != nil {
		log.Println("handle new client error:", err)
		client.conn.Close()
		return
	}

}

// 公共的处理rpc请求
func (c *Client) handleRequest(req *MyPack, codec Codec) {
	// 查找处理函数
	handler, ok := c.handlerMap[req.MethodName]
	if !ok {
		log.Println("No handler found for", req.MethodName)
		// 创建一个错误响应
		res := &MyPack{
			ID:    req.ID,
			Type:  ResponseType,
			Error: fmt.Sprintf("No handler found for %s", req.MethodName),
		}
		// 发送错误响应
		err := c.send(res)
		if err != nil {
			c.errCh <- err
			return
		}
		return
	}

	// 解析参数数组
	var rawArgs []interface{}
	err := json.Unmarshal([]byte(req.Args), &rawArgs)
	if err != nil {
		log.Println("Failed to unmarshal args:", err)
		return
	}

	// 反射调用处理函数
	funcValue := reflect.ValueOf(handler)
	funcType := funcValue.Type()

	// Prepare the arguments for the function call
	in := make([]reflect.Value, funcType.NumIn()) // Use the number of parameters of the function

	// Convert raw arguments to the correct type
	for i := 0; i < funcType.NumIn(); i++ {
		argType := funcType.In(i)
		rawArg := rawArgs[i]

		// If the argument is a slice of int
		if argType.Kind() == reflect.Slice && argType.Elem().Kind() == reflect.Int {
			rawSlice, ok := rawArg.([]interface{})
			if !ok {
				log.Println("Failed to convert argument to []interface{}:", rawArg)
				return
			}

			intSlice := make([]int, len(rawSlice))
			for i, v := range rawSlice {
				num, ok := v.(float64) // json.Unmarshal converts numbers to float64
				if !ok {
					log.Println("Failed to convert argument to int:", v)
					return
				}
				intSlice[i] = int(num)
			}

			in[i] = reflect.ValueOf(intSlice)
		} else if argType.Kind() == reflect.Int {
			num, ok := rawArg.(float64) // json.Unmarshal converts numbers to float64
			if !ok {
				log.Println("Failed to convert argument to int:", rawArg)
				return
			}

			in[i] = reflect.ValueOf(int(num))
		} else if argType.Kind() == reflect.Float64 { // Handle float64 type
			num, ok := rawArg.(float64)
			if !ok {
				log.Println("Failed to convert argument to float64:", rawArg)
				return
			}

			in[i] = reflect.ValueOf(num)
		} else if argType.Kind() == reflect.String { // Handle string type
			str, ok := rawArg.(string)
			if !ok {
				log.Println("Failed to convert argument to string:", rawArg)
				return
			}

			in[i] = reflect.ValueOf(str)
		} else if argType.Kind() == reflect.Bool { // Handle bool type
			b, ok := rawArg.(bool)
			if !ok {
				log.Println("Failed to convert argument to bool:", rawArg)
				return
			}

			in[i] = reflect.ValueOf(b)
		} else if argType.Kind() == reflect.Ptr && argType.Elem().Kind() == reflect.String { // Handle *string type
			str, ok := rawArg.(string)
			if !ok {
				log.Println("Failed to convert argument to string:", rawArg)
				return
			}

			in[i] = reflect.ValueOf(&str)
		} else // Handle CustomType
		if argType.Kind() == reflect.Struct {
			rawMap, ok := rawArg.(map[string]interface{})
			if !ok {
				log.Println("Failed to convert argument to map[string]interface{}:", rawArg)
				return
			}

			// Create a new instance of the struct
			strctPtr := reflect.New(argType)
			strct := strctPtr.Elem()

			// Iterate over all fields of the struct
			for i := 0; i < strct.NumField(); i++ {
				field := strct.Field(i)
				fieldType := strct.Type().Field(i)

				// Get the value from the map
				rawValue, ok := rawMap[fieldType.Name]
				if !ok {
					log.Println("Missing field in map:", fieldType.Name)
					return
				}

				// Convert the raw value to the correct type and set the field
				switch field.Kind() {
				case reflect.Int:
					value, ok := rawValue.(float64)
					if !ok {
						log.Println("Failed to convert value to float64:", rawValue)
						return
					}
					field.SetInt(int64(value))
				case reflect.String:
					value, ok := rawValue.(string)
					if !ok {
						log.Println("Failed to convert value to string:", rawValue)
						return
					}
					field.SetString(value)
				// Add more cases here for other types
				default:
					log.Println("Unsupported field type:", field.Kind())
					return
				}
			}

			in[i] = strct
		} else {
			// Handle other types...
		}
	}

	out := funcValue.Call(in)

	// 将处理函数的参数从反射类型转换回实际的值
	argsInterface := make([]interface{}, len(in))
	for i, v := range in {
		argsInterface[i] = v.Interface()
	}

	// 将参数序列化为JSON字符串
	argsJson, err := json.Marshal(argsInterface)
	if err != nil {
		// 处理错误
	}

	// 将处理函数的返回值从反射类型转换回实际的值
	outInterface := make([]interface{}, len(out))
	for i, v := range out {
		outInterface[i] = v.Interface()
	}

	// 将返回值序列化为JSON字符串
	outJson, err := json.Marshal(outInterface)
	if err != nil {
		// 处理错误
	}

	// 创建响应对象
	res := &MyPack{
		ID:     req.ID,
		Type:   ResponseType,
		Args:   argsJson, // 添加参数
		Result: outJson,  // 使用序列化后的返回值
	}

	err = c.send(res)
	if err != nil {
		c.errCh <- err
		return
	}
}

// 服务端提供给客户的rpc
func (c *Client) handleConnection(conn net.Conn) {
	codec := NewTransparentCodec()
	for {
		req := &MyPack{}
		err := c.receive(req)
		if err != nil {
			c.errCh <- err
			return
		}
		c.handleRequest(req, codec)
	}
}

// 提供给服务端调用
func (c *Client) HandleServerRequest() {
	c.pool.Start()
	go func() {
		for pack := range c.serverReqCh {
			c.handleRequest(pack, c.codec)
		}
	}()
}

func (c *Client) sendRequests() {
	defer func() {
		close(c.sendCh)
	}()
	for req := range c.sendCh {
		err := c.send(req)
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
		err := c.receive(res)
		if err != nil {
			c.errCh <- err
			return
		}
		switch res.Type {
		case ResponseType:
			value, ok := c.resChs.Load(res.ID)
			if ok {
				ch := value.(chan *MyPack)
				ch <- res
				c.resChs.Delete(res.ID)
			} else {
				c.errCh <- fmt.Errorf("no channel found for response ID %d", res.ID)
			}
		case RequestType:
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
		Args:       value,
	}

	// 发送请求并获取响应
	res, err := c.Call(req)
	if err != nil {
		return &InvokeResult{Err: err}
	}
	// 检查响应中的错误字段
	if res.Error != "" {
		// 如果有错误，创建一个包含错误信息的InvokeResult并返回
		return &InvokeResult{Err: errors.New(res.Error)}
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
		Result: []byte(value),
	}
	c.sendCh <- res
	return nil
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

	_, err := NewServer("127.0.0.1:6688", func(c *Client) error {
		c.RegisterHandler("ToUpper", ToUpper)
		c.RegisterHandler("Add", Add)
		c.RegisterHandler("Add2", Add2)
		c.RegisterHandler("TestRPCFunc", TestRPCFunc)
		remotestub := Lpcstub{
			client: c,
		}

		fmt.Printf("=====新的客户端接入=====\r\n")
		fmt.Printf("使用桩回调客户端rpc过程Add2\r\n")
		ret1, ret2, err := remotestub.Add2(1, 2)
		if err != nil {
			fmt.Printf("发生错误\r\n")
			return nil
		}
		fmt.Printf("ret1,2:%d %d", ret1, ret2)

		return nil
	})
	if err != nil {
		log.Fatal(err)
	}

	client, err := Dial("127.0.0.1:6688")
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	client.RegisterHandler("ToUpper", ToUpper)
	client.RegisterHandler("Add", Add)
	client.RegisterHandler("Add2", Add2)
	client.RegisterHandler("TestRPCFunc", TestRPCFunc)

	client.HandleServerRequest() // 启动处理服务端请求的goroutine

	for i := 0; i < 16666; i++ {

		remotestub := Lpcstub{
			client: client,
		}

		ret1, err := remotestub.Add(1, 2)
		if err != nil {
			return
		}
		fmt.Printf("ret:%d\r\n", ret1)

		arg1 := 10
		arg2 := 20.0
		arg3 := "hello"
		arg4 := true
		arg5 := []int{1, 2, 3}
		arg6 := "world"
		arg7 := CustomType{
			Field1: 100,
			Field2: "foo",
		}

		res1, res2, res3, res4, res5, res6, res7, err, err2 := remotestub.TestRPCFunc(arg1, arg2, arg3, arg4, arg5, &arg6, arg7)

		if err2 != nil {
			return
		}
		if err != nil {
			fmt.Println("Error:", err)
			return
		}

		fmt.Println("Result:", res1, *res2, res3, res4, res5, *res6, res7)

		//time.Sleep(66666)
	}

	//GenerateWrappers(client.GenerateDocs())

	time.Sleep(666 * time.Second)
}
