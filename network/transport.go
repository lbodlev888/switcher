package network

import (
	"fmt"
	"io"
	"net"
)

func SendData(conn net.Conn, data []byte) error {
	if len(data) > 255 {
		return fmt.Errorf("sendData: packet too large(%d, max 255)", len(data))
	}
	
	buf := make([]byte, 0, 1 + len(data))
	buf = append(buf, byte(len(data)))
	buf = append(buf, data...)

	_, err := conn.Write(buf)
	return err
}

func ReadData(conn net.Conn) ([]byte, error) {
	var lenBuf [1]byte
	if _, err := io.ReadFull(conn, lenBuf[:]); err != nil {
		return nil, fmt.Errorf("readData: reading length: %w", err)
	}

	n := int(lenBuf[0])
	if n == 0 {
		return []byte{}, nil
	}
	
	data := make([]byte, n)
	if _, err := io.ReadFull(conn, data); err != nil {
		return nil, fmt.Errorf("readData: reading paylod: %w", err)
	}

	return data, nil
}

func Relay(dst, src io.ReadWriteCloser) {
	done := make(chan struct{}, 2)

	cp := func(dst, src io.ReadWriteCloser) {
		io.Copy(dst, src)
		dst.Close()
		done <- struct{}{}
	}

	go cp(dst, src)
	go cp(src, dst)

	<-done
	<-done
}
