package main

import (
	"bytes"
	"context"
	flexpacketprotocol "devdemo/protocol"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/1370339317/websocketstream"
	cbor "github.com/fxamacker/cbor/v2"

	"github.com/vmihailenco/msgpack"
)

// -序列化反序列化工厂--------------------------------------------------------------------------------------------------------------

type CodecFactory interface {
	Create() Serializer
}

type MsgpackCodecFactory struct{}
type GobCodecFactory struct{}
type CborCodecFactory struct{}
type JsonCodecFactory struct{}

func (f *MsgpackCodecFactory) Create() Serializer {
	return &MsgpackCodec{}
}

func (f *GobCodecFactory) Create() Serializer {
	return &GobCodec{}
}

func (f *CborCodecFactory) Create() Serializer {
	return &CborCodec{}
}

func (f *JsonCodecFactory) Create() Serializer {
	return &JsonCodec{}
}

type MsgpackCodec struct{}
type GobCodec struct{}
type CborCodec struct{}
type JsonCodec struct{}

func (c *MsgpackCodec) Marshal(v interface{}) ([]byte, error) {
	return msgpack.Marshal(v)
}

func (c *MsgpackCodec) Unmarshal(data []byte, v interface{}) error {
	return msgpack.Unmarshal(data, v)
}

func (c *GobCodec) Marshal(v interface{}) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(v)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (c *GobCodec) Unmarshal(data []byte, v interface{}) error {
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	return dec.Decode(v)
}

func (c *CborCodec) Marshal(v interface{}) ([]byte, error) {
	return cbor.Marshal(v)
}

func (c *CborCodec) Unmarshal(data []byte, v interface{}) error {
	return cbor.Unmarshal(data, v)
}

func (c *JsonCodec) Marshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func (c *JsonCodec) Unmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

// -序列化反序列化工厂--------------------------------------------------------------------------------------------------------------

// -数据转换与容错--------------------------------------------------------------------------------------------------------------

func (tc *Client) TypeNameToType(typeName string) reflect.Type {
	switch typeName {
	case "int":
		return reflect.TypeOf(int(0))
	case "int64":
		return reflect.TypeOf(int64(0))
	case "float64":
		return reflect.TypeOf(float64(0))
	case "string":
		return reflect.TypeOf("")
	case "bool":
		return reflect.TypeOf(true)
	case "*float64":
		var p *float64
		return reflect.TypeOf(p)
	case "*string":
		var p *string
		return reflect.TypeOf(p)
	case "[]int":
		return reflect.TypeOf([]int{})
	case "error":
		return reflect.TypeOf(errors.New(""))
	default:
		fmt.Printf("unsupported type: %s\r\n" + typeName)
	}
	return nil
}

func convertBasicType(val interface{}, targetType reflect.Type) interface{} {
	switch targetType.Kind() {
	case reflect.Int:
		switch v := val.(type) {
		case float64:
			if v >= float64(math.MinInt) && v <= float64(math.MaxInt) {
				return int(v)
			}
		case int:
			return v
		case int8:
			return int(v)
		case int16:
			return int(v)
		case int32:
			return int(v)
		case int64:
			if v >= int64(math.MinInt) && v <= int64(math.MaxInt) {
				return int(v)
			}
		case uint64:
			if v <= uint64(math.MaxInt) {
				return int(v)
			}
		default:
			fmt.Printf("unsupported type: %T", val)
			return nil
		}
	case reflect.Int64:
		switch v := val.(type) {
		case float64:
			return int64(v)
		case int:
			return int64(v)
		case int64:
			return v
		case uint64:
			if v <= uint64(math.MaxInt64) {
				return int64(v)
			}
		default:
			fmt.Printf("unsupported type: %T", val)
			return nil
		}
	case reflect.Uint:
		switch v := val.(type) {
		case float64:
			if v >= 0 {
				return uint(v)
			}
		case uint:
			return v
		case uint64:
			return uint(v)
		default:
			fmt.Printf("unsupported type: %T", val)
			return nil
		}
	case reflect.Uint64:
		switch v := val.(type) {
		case float64:
			if v >= 0 {
				return uint64(v)
			}
		case uint:
			return uint64(v)
		case uint64:
			return v
		default:
			fmt.Printf("unsupported type: %T", val)
			return nil
		}
	case reflect.Float32:
		switch v := val.(type) {
		case float64:
			return float32(v)
		case int:
			return float32(v)
		case int64:
			return float32(v)
		default:
			fmt.Printf("unsupported type: %T", val)
			return nil
		}
	case reflect.Float64:
		switch v := val.(type) {
		case float64:
			return v
		case int:
			return float64(v)
		case int64:
			return float64(v)
		default:
			fmt.Printf("unsupported type: %T", val)
			return nil
		}
	case reflect.String:
		if v, ok := val.(string); ok {
			return v
		} else {
			fmt.Printf("unsupported type: %T", val)
			return nil
		}
	case reflect.Bool:
		if v, ok := val.(bool); ok {
			return v
		} else {
			fmt.Printf("unsupported type: %T", val)
			return nil
		}
	default:
		fmt.Printf("unsupported type: %s", targetType.String())
		return nil
	}
	return nil
}

func (tc *Client) ConvertToType(val interface{}, targetType reflect.Type) interface{} {
	if val == nil {
		return nil
	}
	switch targetType.Kind() {
	case reflect.Ptr: // Handle pointers
		elemVal := tc.ConvertToType(val, targetType.Elem())
		ptr := reflect.New(targetType.Elem())
		ptr.Elem().Set(reflect.ValueOf(elemVal))
		return ptr.Interface()
	case reflect.Slice: // Handle slices
		slice := reflect.MakeSlice(targetType, 0, 0)
		v := reflect.ValueOf(val)
		for i := 0; i < v.Len(); i++ {
			elem := v.Index(i).Interface()
			slice = reflect.Append(slice, reflect.ValueOf(tc.ConvertToType(elem, targetType.Elem())))
		}
		return slice.Interface()
	case reflect.Map: // Handle maps
		if v, ok := val.(map[string]interface{}); ok {
			m := reflect.MakeMap(targetType)
			for k, elem := range v {
				m.SetMapIndex(reflect.ValueOf(k), reflect.ValueOf(tc.ConvertToType(elem, targetType.Elem())))
			}
			return m.Interface()
		}
	case reflect.Struct:
		m := make(map[string]interface{})
		switch v := val.(type) {
		case map[string]interface{}:
			m = v
		case map[interface{}]interface{}:
			for k, v := range v {
				if ks, ok := k.(string); ok {
					m[ks] = v
				} else {
					fmt.Printf("unsupported key type: %T", k)
					return nil
				}
			}
		default:
			valValue := reflect.ValueOf(val)
			valType := valValue.Type()
			for i := 0; i < valValue.NumField(); i++ {
				fieldName := valType.Field(i).Name
				fieldValue := valValue.Field(i).Interface()
				m[fieldName] = fieldValue
			}
		}

		targetVal := reflect.New(targetType).Elem()
		for i := 0; i < targetVal.NumField(); i++ {
			fieldName := targetType.Field(i).Name
			fieldValue := targetVal.Field(i)

			if !fieldValue.CanSet() {
				continue
			}

			mapValue, ok := m[fieldName]
			if !ok {
				continue
			}

			converted := tc.ConvertToType(mapValue, fieldValue.Type())
			if converted == nil {
				continue
			}

			fieldValue.Set(reflect.ValueOf(converted))
		}

		return targetVal.Interface()

	case reflect.Interface: // Handle error interface
		if targetType.Name() == "error" {
			if v, ok := val.(string); ok {
				return errors.New(v)
			}
		}
	default: // Handle basic types
		return convertBasicType(val, targetType)
	}
	return nil
}

//-数据转换与容错--------------------------------------------------------------------------------------------------------------

type Serializer interface {
	Marshal(interface{}) ([]byte, error)
	Unmarshal([]byte, interface{}) error
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
	conn        io.ReadWriter
	codec       Serializer
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

// 发送协议帧
func (c *Client) writeToConn(data []byte) error {
	_, err := c.frameWriter.Write(data)
	return err
}

// 读取协议帧
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

func PrintHex(data []byte) {
	fmt.Print("unsigned char data[] = {")
	for i, byte := range data {
		if i != 0 {
			fmt.Print(", ")
		}
		fmt.Printf("0x%02x", byte)
	}
	fmt.Println("};")
	fmt.Printf("% x", data)
}

// 序列化逻辑结构并发送到帧协议层
func (c *Client) send(v *MyPack) error {
	data, err := c.codec.Marshal(v)

	if err != nil {
		return err
	}

	return c.writeToConn(data)
}

// 从帧协议层读取报文并反序列化
func (c *Client) receive(v *MyPack) error {
	data, err := c.readFromConn()
	if err != nil {
		return err
	}
	return c.codec.Unmarshal(data, v)
}

// 创建客户端，指定底层流数据的出入口，序列化反序列化类方式
func NewClient(conn io.ReadWriter, codecFactory CodecFactory) (*Client, error) {

	pool := NewWorkerPool(10, nil) // 创建一个没有处理函数的WorkerPool
	//报文协议
	protocol := flexpacketprotocol.New(conn, []byte("aacc"), []byte("eezz"))
	client := &Client{
		conn:        conn,
		codec:       codecFactory.Create(),
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
	clients      []*Client //客户端们
	onNewClient  ClientHandler
	CodecFactory CodecFactory
}

//创建rpc服务端
//①此处回调,用于监听逻辑.当有客户端成功连入后请调用s.AcceptConnections(conn)
//②此处回调,当客户端成功连入后,进行的操作.如为其绑定函数等.属于逻辑函数
func NewServer(ListenCallBack func(s *Server) error, onNewClientCallBack func(*Client) error, codecFactory CodecFactory) (*Server, error) {

	server := &Server{
		clients:      make([]*Client, 0),
		onNewClient:  onNewClientCallBack,
		CodecFactory: codecFactory,
	}

	go ListenCallBack(server)

	return server, nil
}
func (s *Server) Close() error {
	return nil
}
func (s *Server) NewClient(conn io.ReadWriter) *Client {

	pool := NewWorkerPool(10, nil)
	protocol := flexpacketprotocol.New(conn, []byte("aacc"), []byte("eezz"))
	client := &Client{
		conn:        conn,
		codec:       s.CodecFactory.Create(),
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

//传入对等的服务端通信接口对象
func (s *Server) AcceptConnections(conn io.ReadWriter) {
	client := s.NewClient(conn)
	s.clients = append(s.clients, client)
	s.handleNewClient(client)
}

// 新链接接入时的动作触发
func (s *Server) handleNewClient(client *Client) {

	// 在这里处理新的连接
	err := s.onNewClient(client) // 增加这一行
	if err != nil {
		log.Println("handle new client error:", err)
		return
	}

}

// 参数转换
func (c *Client) convertArg(arg interface{}, argType reflect.Type) (reflect.Value, error) {
	converted := c.ConvertToType(arg, argType)
	if converted == nil {
		return reflect.Value{}, fmt.Errorf("failed to convert argument: %v", arg)
	}
	return reflect.ValueOf(converted), nil
}

// 客户端xiecheng
func (c *Client) handleRequest(req *MyPack) {

	// 创建响应对象
	res := &MyPack{
		ID:   req.ID,
		Type: ResponseType,
	}

	var err error
	// 查找处理函数
	handler, ok := c.handlerMap[req.MethodName]
	if ok {
		// 解析参数数组
		var rawArgs []interface{}
		err := c.codec.Unmarshal([]byte(req.Args), &rawArgs)
		if err == nil {

			// 反射调用处理函数
			funcValue := reflect.ValueOf(handler)
			funcType := funcValue.Type()

			// Prepare the arguments for the function call
			in := make([]reflect.Value, funcType.NumIn()) // Use the number of parameters of the function

			// Convert raw arguments to the correct type
			for i := 0; i < funcType.NumIn(); i++ {
				argType := funcType.In(i) // Convert raw arguments to the correct type
				in[i], err = c.convertArg(rawArgs[i], argType)
				if err != nil {
					break
				}
			}

			//无错误才走
			if err == nil {
				out := funcValue.Call(in)

				// 将处理函数的参数从反射类型转换回实际的值
				argsInterface := make([]interface{}, len(in))
				for i, v := range in {
					argsInterface[i] = v.Interface()
				}

				// 将参数序列化为字节数组
				argsbytes, err := c.codec.Marshal(argsInterface)
				if err == nil {

					res.Args = argsbytes

					// 将处理函数的返回值从反射类型转换回实际的值
					outInterface := make([]interface{}, len(out))
					for i, v := range out {
						outInterface[i] = v.Interface()
					}

					// 将返回值序列化为字节数组
					outbytes, err := c.codec.Marshal(outInterface)
					if err == nil {
						res.Result = outbytes
					} else {
						// 处理错误
						res.Error = fmt.Sprintf("outInterface Marshal fail: %s", err.Error())
					}

				} else {
					// 处理错误
					res.Error = fmt.Sprintf("argsInterface Marshal fail: %s", err.Error())
				}

			} else {
				// 处理错误
				res.Error = fmt.Sprintf("argsin UnMarshal fail: %s", err.Error())
			}
		} else {
			log.Println("Failed to unmarshal args:", err)
			return
		}

	} else {
		res.Error = fmt.Sprintf("No handler found for %s", req.MethodName)
	}

	err = c.send(res)
	if err != nil {
		c.errCh <- err
		return
	}
}

// 客户端接受调用协程
func (c *Client) HandleServerRequest() {
	c.pool.Start()
	go func() {
		for pack := range c.serverReqCh {
			c.handleRequest(pack)
		}
	}()
}

// 客户端发送协程
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

// 客户端接收协程
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

// 客户端请求远端
func (c *Client) RpcRequest(req *MyPack, timeout time.Duration) (*MyPack, error) {
	req.Type = RequestType // Call方法
	req.ID = atomic.AddUint64(&c.seq, 1)
	resCh := make(chan *MyPack)
	c.resChs.Store(req.ID, resCh)
	c.sendCh <- req

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	select {
	case res := <-resCh:
		return res, nil
	case err := <-c.errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err() // 如果超时或者被取消，返回一个错误
	}

}

// 设置引用参的值
func (c *Client) setArgValue(v reflect.Value, result interface{}) error {
	if v.Kind() == reflect.Ptr {
		converted := c.ConvertToType(result, v.Elem().Type())
		v.Elem().Set(reflect.ValueOf(converted))
	} else if v.Kind() == reflect.Slice {
		converted := c.ConvertToType(result, v.Type())
		reflect.Copy(v, reflect.ValueOf(converted))
	}
	return nil
}

// 客户端对rpc进行Invoke调用，带超时
func (c *Client) InvokeWithTimeout(method string, timeout time.Duration, args ...interface{}) *InvokeResult {
	// 将参数序列化为字节数组
	value, err := c.codec.Marshal(args)
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
	res, err := c.RpcRequest(req, timeout)
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
	err = c.codec.Unmarshal([]byte(res.Args), &result)
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
		err := c.setArgValue(reflect.ValueOf(arg), result[i])
		if err != nil {
			invokeResult.Err = err
			break
		}
	}

	return invokeResult
}

// 客户端对rpc进行Invoke调用
func (c *Client) Invoke(method string, args ...interface{}) *InvokeResult {
	return c.InvokeWithTimeout(method, time.Second*15, args...)
}

// 客户端注册rpc例程
func (c *Client) RegisterHandler(name string, handler HandlerFunc) {
	v := reflect.ValueOf(handler)
	if v.Kind() != reflect.Func {
		fmt.Printf("Handler is not a function: %s\r\n", name)
		return
	}
	c.handlerMap[name] = handler
}

func tttclient(factory CodecFactory) {

	conn, err := net.Dial("tcp", "127.0.0.1:6688")
	if err != nil {
		return
	}
	client, err := NewClient(conn, factory)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	client.RegisterHandler("ToUpper", ToUpper)
	client.RegisterHandler("Add", Add)
	client.RegisterHandler("Add2", Add2)
	client.RegisterHandler("TestRPCFunc", TestRPCFunc)

	client.HandleServerRequest() // 启动处理服务端请求的goroutine
	start := time.Now()          // 获取当前时间
	for i := 0; i < 10000; i++ {

		remotestub := Lpcstub1{
			client: client,
		}

		fmt.Printf("=====新的客户端接入=====\r\n")
		fmt.Printf("使用桩回调客户端rpc过程Add2\r\n")

		startTime := time.Now() // 获取开始时间

		for i := 0; i < 10000; i++ {
			a := 1
			b := 1
			client.Invoke("Add", &a, b)
			ret1, err := remotestub.Add(1, 2)
			if err != nil {
				fmt.Printf("发生错误%d\r\n", ret1)
				return
			}
			//fmt.Printf("ret1,2:%d %d\r\n", ret1)
		}

		elapsed := time.Since(startTime) // 计算从startTime到现在所经过的时间
		fmt.Printf("代码块运行时间: %s\n", elapsed)

		// ret1, err := remotestub.Add(1, 2)
		// if err != nil {
		// 	return
		// }
		// fmt.Printf("ret:%d\r\n", ret1)

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
		arg8 := CustomType{
			Field1: 666,
			Field2: "dqq",
		}

		// res1, res2, res3, res4, res5, res6, res7, res8, err, err2 := remotestub.TestRPCFunc(arg1, arg2, arg3, arg4, arg5, &arg6, arg7, &arg8)
		_, _, _, _, _, _, _, _, err, err2 := remotestub.TestRPCFunc(arg1, arg2, arg3, arg4, arg5, &arg6, arg7, &arg8)

		if err2 != nil {
			return
		}
		if err != nil {
			fmt.Println("Error:", err)
			return
		}

		// fmt.Println("Result:", res1, *res2, res3, res4, res5, *res6, res7, res8)

		//time.Sleep(66666)
	}
	elapsed := time.Since(start) // 计算经过的时间

	fmt.Printf("The code executed in %s\r\n", elapsed)
	//GenerateWrappers(client.GenerateDocs())
}

func MarshelspeedTest() {
	startTime := time.Now() // 获取开始时间

	var dddd []byte
	for i := 0; i < 100000; i++ {

		v := MyPack{ID: 6}
		da, e := msgpack.Marshal(v)
		if e == nil {
			dddd = da
		}
	}

	print(dddd)

	elapsed := time.Since(startTime) // 计算从startTime到现在所经过的时间
	fmt.Printf("代码块运行时间: %s\n", elapsed)
}

func main() {

	factory := &MsgpackCodecFactory{}

	gob.Register(CustomType{})
	_, err := NewServer(func(s *Server) error {
		listener, err := net.Listen("tcp", "127.0.0.1:6688")
		if err != nil {
			return err
		}
		for {
			conn, err := listener.Accept()
			if err != nil {
				log.Println("Accept error:", err)
				continue
			}
			s.AcceptConnections(conn)

		}
	}, func(c *Client) error {
		c.RegisterHandler("ToUpper", ToUpper)
		c.RegisterHandler("Add", Add)
		c.RegisterHandler("Add2", Add2)
		c.RegisterHandler("TestRPCFunc", TestRPCFunc)

		if false {
			remotestub := Lpcstub1{
				client: c,
			}

			fmt.Printf("=====新的客户端接入=====\r\n")
			fmt.Printf("使用桩回调客户端rpc过程Add2\r\n")
			ret1, ret2, err := remotestub.Add2(1, 2)
			if err != nil {
				fmt.Printf("发生错误\r\n")
				return nil
			}
			fmt.Printf("ret1,2:%d %d\r\n", ret1, ret2)
		}

		return nil
	}, factory)
	if err != nil {
		log.Fatal(err)
	}

	for i := 0; i < 4; i++ {
		tttclient(factory)
	}

	time.Sleep(66666 * time.Second)
}
