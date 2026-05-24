package network

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
)

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

func EncodeDestination(dstAddr string) ([]byte, error) {
	buf := make([]byte, 0, 6) //4 bytes for IP address and 2 bytes for dst port

	destination := strings.Split(dstAddr, ":") //0 for IP, 1 for Port

	dstIP := net.ParseIP(destination[0]).To4()

	if dstIP == nil {
		return nil, fmt.Errorf("encodeDestination: invalid dstIP: dst %q", destination[0])
	}

	dstPort, err := strconv.ParseUint(destination[1], 10, 16)
	if err != nil {
		return nil, fmt.Errorf("encodeDestination: invalid destination port: %w", err)
	}

	buf = append(buf, dstIP...)
	buf = binary.BigEndian.AppendUint16(buf, uint16(dstPort))
	return buf, nil
}

func DecodeDestination(buf []byte) (string, error) {
	if len(buf) != 6 {
		return "", fmt.Errorf("decodeDestionation: invalid buf length: required 6 got %d", len(buf))
	}

	addr := net.IP(buf[:4]).To4()

	port := binary.BigEndian.Uint16(buf[4:6])

	return fmt.Sprintf("%s:%d", addr.String(), port), nil
}
