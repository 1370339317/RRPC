package flexpacketprotocol

import (
	"bytes"
	"fmt"
	"testing"
)

func TestFlexPacketProtocol(t *testing.T) {
	header := []byte("HEADER")
	footer := []byte("FOOTER")

	header = []byte{0x56, 0x78, 0x89}
	footer = []byte{0x45, 0x67, 0x34}
	// 测试数据
	testData := []byte("Hello, world!")

	t.Run("TestWriteAndRead", func(t *testing.T) {
		buffer := new(bytes.Buffer)
		fpp := New(buffer, header, footer)

		n, err := fpp.Write(testData)
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}

		// 输出16进制的数据
		for _, b := range buffer.Bytes() {
			fmt.Printf("%02x ", b)
		}
		fmt.Println()

		if n != len(testData) {
			t.Fatalf("Write returned wrong count: got %v, want %v", n, len(testData))
		}

		readData := make([]byte, len(testData))
		n, err = fpp.Read(readData)
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}

		if n != len(testData) {
			t.Fatalf("Read returned wrong count: got %v, want %v", n, len(testData))
		}

		if !bytes.Equal(readData, testData) {
			t.Fatalf("Read returned wrong data: got %v, want %v", readData, testData)
		}
	})

	t.Run("TestSmallBuffer", func(t *testing.T) {
		buffer := new(bytes.Buffer)
		fpp := New(buffer, header, footer)
		fpp.Write(testData)

		smallBuffer := make([]byte, len(testData)-1)
		_, err := fpp.Read(smallBuffer)
		if err == nil {
			t.Fatalf("Expected error when buffer is too small")
		}
	})

	t.Run("TestChecksumError", func(t *testing.T) {
		buffer := new(bytes.Buffer)
		fpp := New(buffer, header, footer)
		fpp.Write(testData)

		buffer.Bytes()[len(header)+4+len(testData)/2]++ // 改变数据，使校验和错误
		readData := make([]byte, len(testData))
		_, err := fpp.Read(readData)
		if err == nil {
			t.Fatalf("Expected error when checksum is incorrect")
		}
	})

	t.Run("TestFooterError", func(t *testing.T) {
		buffer := new(bytes.Buffer)
		fpp := New(buffer, header, footer)
		fpp.Write(testData)

		buffer.Bytes()[len(buffer.Bytes())-len(footer)/2]++ // 改变帧尾，使其错误
		readData := make([]byte, len(testData))
		_, err := fpp.Read(readData)
		if err == nil {
			t.Fatalf("Expected error when footer is incorrect")
		}
	})

	// 添加更多的测试用例...
}
