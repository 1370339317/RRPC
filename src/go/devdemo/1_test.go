package main

import (
	"testing"
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

func benchmarkTestRPCFunc(b *testing.B, factory CodecFactory, concurrency int) {
	b.SetParallelism(concurrency) // 设置并发级别
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		client, err := Dial("127.0.0.1:6688", factory)
		if err != nil {
			b.Fatal(err)
		}

		remotestub := Lpcstub1{
			client: client,
		}

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

		for pb.Next() {
			_, _, _, _, _, _, _, _, err, err2 := remotestub.TestRPCFunc(arg1, arg2, arg3, arg4, arg5, &arg6, arg7, &arg8)
			if err2 != nil || err != nil {
				b.Fatal(err2, err)
			}
		}
	})
}

// ... 你的基准测试函数 ...

func BenchmarkTestRPCFunc1(b *testing.B) {
	benchmarkTestRPCFunc(b, factory, 1)
}

func BenchmarkTestRPCFunc500(b *testing.B) {
	benchmarkTestRPCFunc(b, factory, 500)
}

func BenchmarkTestRPCFunc2000(b *testing.B) {
	benchmarkTestRPCFunc(b, factory, 2000)
}

func BenchmarkTestRPCFunc5000(b *testing.B) {
	benchmarkTestRPCFunc(b, factory, 5000)
}

func BenchmarkTestRPCFunc10000(b *testing.B) {
	benchmarkTestRPCFunc(b, factory, 10000)
}

func BenchmarkTestRPCFunc100000(b *testing.B) {
	benchmarkTestRPCFunc(b, factory, 100000)
}
