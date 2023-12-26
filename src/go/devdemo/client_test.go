package main

import (
	"testing"
)

func TestClient(t *testing.T) {

	// if true {
	// 	client, err := Dial("127.0.0.1:6688")
	// 	if err != nil {
	// 		fmt.Println("Error:", err)
	// 		return
	// 	}

	// 	client.RegisterHandler("ToUpper", ToUpper)
	// 	client.RegisterHandler("Add", Add)
	// 	client.RegisterHandler("Add2", Add2)

	// 	client.HandleServerRequest() // 启动处理服务端请求的goroutine

	// 	for i := 0; i < 6; i++ {

	// 		remotestub := Lpcstub{
	// 			client: client,
	// 		}

	// 		for i := 0; i < 5; i++ {
	// 			fmt.Printf("循环调用服务端rpc过程Add\r\n")
	// 			ret1, err := remotestub.Add(1, 2)
	// 			if err != nil {
	// 				return
	// 			}
	// 			fmt.Printf("ret1,2:%d", ret1)
	// 			time.Sleep(1 * time.Second)
	// 		}

	// 	}
	// }
	//return
	client, err := Dial("127.0.0.1:8080")
	if err != nil {
		t.Fatal(err)
	}

	result := client.Invoke("ToUpper", "hello")
	if result.Err != nil {
		t.Fatal(result.Err)
	}

}
