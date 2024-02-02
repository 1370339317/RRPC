package main

import (
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var server *Server
var factory CodecFactory = &JsonCodecFactory{}

func TestMain(m *testing.M) {
	var err error
	server, err = NewServer("127.0.0.1:6688", func(c *Client) error {
		c.RegisterHandler("ToUpper", ToUpper)
		c.RegisterHandler("Add", Add)
		c.RegisterHandler("Add2", Add2)
		c.RegisterHandler("TestRPCFunc", TestRPCFunc)
		return nil
	}, factory) // 使用你的默认编解码器
	if err != nil {
		panic(err)
	}
	defer server.Close()

	m.Run()
}

func BenchmarkRPC(b *testing.B) {
	concurrencyLevels := []int{1, 2, 4, 8, 16, 32, 64, 128, 256} // 并发级别

	for _, concurrency := range concurrencyLevels {
		b.Run("Concurrency"+strconv.Itoa(concurrency), func(b *testing.B) {
			var counter int64      // 原子计数器
			wg := sync.WaitGroup{} // 用于同步所有 goroutine
			wg.Add(concurrency)

			start := time.Now() // 记录开始时间

			for i := 0; i < concurrency; i++ {
				go func() {
					defer wg.Done()

					conn, err := net.Dial("tcp", "127.0.0.1:6688")
					if err != nil {
						return
					}

					client, err := NewClient(conn, factory)
					if err != nil {
						b.Fatal(err)
					}

					remotestub := Lpcstub1{
						client: client,
					}

					for {
						if atomic.AddInt64(&counter, 1) > int64(b.N) {
							break
						}
						// 发送 RPC 请求...

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

						_, _, _, _, _, _, _, _, err, err2 := remotestub.TestRPCFunc(arg1, arg2, arg3, arg4, arg5, &arg6, arg7, &arg8)
						if err2 != nil || err != nil {
							b.Fatal(err2, err)
						}
					}
				}()
			}

			wg.Wait() // 等待所有 goroutine 完成

			elapsed := time.Since(start) // 计算经过的时间

			b.Logf("reqs/sec: %f", float64(counter)/elapsed.Seconds()) // 报告每秒的请求数
		})
	}
}
