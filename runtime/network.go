package runtime

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
)

type WasmNetwork interface {
	SendTask(ctx context.Context, data []byte, addr net.Addr) error
	Listen(ctx context.Context) error
}

type TCPNetwork struct{}

type QUICNetwork struct{}

// TODO: use a built-in method
func ParseNetworkString(addr string) (net.IP, int, error) {
	network := strings.Split(addr, ":")

	if len(network) == 0 {
		return nil, 0, fmt.Errorf("invalid address format: %v. want: ip:port", addr)
	}
	ip := net.ParseIP(network[0])
	port, parseErr := strconv.ParseInt(network[1], 10, 32)
	if parseErr != nil {
		return nil, 0, fmt.Errorf("error parsing port %v: %v", port, parseErr)
	}

	return ip, int(port), nil
}

//func (n *TCPNetwork) Listen(addr string) (net.Listener, error) {
//	ip, port, err := ParseNetworkString(addr)
//	if err != nil {
//		return nil, err
//	}
//	return net.ListenTCP("tcp", &net.TCPAddr{IP: ip, Port: int(port)})
//}
//
//func (n *TCPNetwork) Dial(addr string) (net.Conn, error) {
//	ip, port, err := ParseNetworkString(addr)
//	if err != nil {
//		return nil, err
//	}
//
//	return net.DialTCP("tcp", nil, &net.TCPAddr{IP: ip, Port: int(port)})
//}

//func (n *QUICNetwork) Listen(addr string) (net.Listener, error) {
//	return nil, nil
//}
//
//func (wt *WasmRuntime) ListenQuic() error {
//	ctx := context.Background()
//	tr := quic.Transport{
//		Conn: wt.quicConn,
//	}
//
//	ln, err := tr.Listen(&tls.Config{InsecureSkipVerify: true, NextProtos: []string{"drone-net"}}, &quic.Config{})
//	if err != nil {
//		return fmt.Errorf("error listening quic: %v", err)
//	}
//	defer ln.Close()
//
//	log.Printf("listening quic on %v", wt.quicConn.LocalAddr())
//
//	for {
//		conn, err := ln.Accept(ctx)
//		log.Println("yo I got a message")
//		if err != nil {
//			return err
//		}
//		go func() {
//			packet, err := conn.AcceptStream(ctx)
//			if err != nil {
//				return
//			}
//			var buf []byte
//			n, _ := packet.Read(buf)
//
//			data := buf[:n]
//
//			wasmTask := wt.Decode(data)
//			log.Println("yo I'm decoding")
//
//			if wasmTask.WasmBinaryLength != 0 {
//				wt.Run(wasmTask.WasmBinary)
//			}
//		}()
//	}
//}
