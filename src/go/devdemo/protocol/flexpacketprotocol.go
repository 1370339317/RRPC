package flexpacketprotocol

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
)

type flexPacketProtocol struct {
	conn     io.ReadWriter
	checksum func([]byte) uint32
	header   []byte
	footer   []byte
}

func New(conn io.ReadWriter, header, footer []byte) *flexPacketProtocol {
	return &flexPacketProtocol{
		conn:     conn,
		checksum: crc32Checksum,
		header:   header,
		footer:   footer,
	}
}

func (fpp *flexPacketProtocol) Write(data []byte) (int, error) {
	length := len(fpp.header) + 4 + len(data) + 4 + len(fpp.footer)
	buffer := make([]byte, length)

	copy(buffer[:len(fpp.header)], fpp.header)
	binary.BigEndian.PutUint32(buffer[len(fpp.header):len(fpp.header)+4], uint32(len(data)))
	copy(buffer[len(fpp.header)+4:len(fpp.header)+4+len(data)], data)
	checksum := fpp.checksum(data)
	binary.BigEndian.PutUint32(buffer[len(fpp.header)+4+len(data):len(fpp.header)+8+len(data)], checksum)
	copy(buffer[len(fpp.header)+8+len(data):], fpp.footer)

	_, err := fpp.conn.Write(buffer)
	return len(data), err
}

func (fpp *flexPacketProtocol) Read(data []byte) (int, error) {
	header := make([]byte, len(fpp.header)+4)
	_, err := io.ReadFull(fpp.conn, header)
	if err != nil {
		return 0, err
	}

	length := binary.BigEndian.Uint32(header[len(fpp.header):])
	if len(data) < int(length) {
		return 0, fmt.Errorf("buffer too small")
	}

	n, err := io.ReadFull(fpp.conn, data[:length])
	if err != nil {
		return n, err
	}

	checksumBytes := make([]byte, 4)
	_, err = io.ReadFull(fpp.conn, checksumBytes)
	if err != nil {
		return n, err
	}

	checksum := binary.BigEndian.Uint32(checksumBytes)
	if fpp.checksum(data[:length]) != checksum {
		return n, fmt.Errorf("checksum error")
	}

	footer := make([]byte, len(fpp.footer))
	_, err = io.ReadFull(fpp.conn, footer)
	if err != nil {
		return n, err
	}

	if string(footer) != string(fpp.footer) {
		return n, fmt.Errorf("incorrect frame footer")
	}

	return n, nil
}

// 私有的校验和函数
func crc32Checksum(data []byte) uint32 {
	return crc32.ChecksumIEEE(data)
}
