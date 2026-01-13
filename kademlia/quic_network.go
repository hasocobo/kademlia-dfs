package kademliadfs

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/pion/stun/v3"
	"github.com/quic-go/quic-go"
)

type QUICNetwork struct {
	conn       *net.UDPConn
	pending    map[NodeId]chan rpcResponse
	rpcHandler RpcHandler

	PublicAddr   *net.UDPAddr // sent and received via stun protocol
	publicAddrCh chan *net.UDPAddr

	requestQueue chan UDPRequest
	mu           sync.Mutex
}

func NewQUICNetwork(ip net.IP, port int) (*QUICNetwork, error) {
	udpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: ip, Port: port})
	if err != nil {
		log.Printf("error getting local candidates")
		return nil, fmt.Errorf("error getting local candidates: %v", err)
	}

	//	googleStunURI := &stun.URI{
	//		Scheme:   stun.SchemeTypeSTUN,
	//		Host:     "stun.l.google.com",
	//		Port:     19302,
	//		Username: "kademlia",
	//		Password: "kademlia",
	//		Proto:    stun.ProtoTypeUDP,
	//	}
	log.Printf("Listening on %v\n", udpConn.LocalAddr().String())

	quicNetwork := &QUICNetwork{
		conn:         udpConn,
		pending:      make(map[NodeId]chan rpcResponse),
		publicAddrCh: make(chan *net.UDPAddr, 1),
		requestQueue: make(chan UDPRequest, packetBufferLimit),
	}

	// create n=workerPoolSize workers with timeout
	for range workerPoolSize {
		go quicNetwork.requestHandlerWorker(context.Background())
	}

	return quicNetwork, nil
}

func (network *QUICNetwork) SetHandler(rpcHandler RpcHandler) {
	network.rpcHandler = rpcHandler
}

func (network *QUICNetwork) Listen(ctx context.Context) error {
	defer network.conn.Close()
	log.Printf("listening on quic network: %v", network.conn.LocalAddr())

	tr := &quic.Transport{
		Conn: network.conn,
	}

	// read non quic packets here
	go func() {
		for {
			buf := make([]byte, MaxUDPPacketSize)
			n, addr, err := tr.ReadNonQUICPacket(ctx, buf)
			if err != nil {
				log.Printf("error reading non quic packet: %v", err)
				continue
			}

			log.Printf("I got a message on non quic listener")

			// TODO: move the following to a seperate goroutine
			data := buf[:n]

			if stun.IsMessage(data) {
				msg := &stun.Message{Raw: data}

				if err := msg.Decode(); err != nil {
					log.Printf("error decoding stun packet: %v", err)
					continue
				}

				var xorAddr stun.XORMappedAddress
				if getErr := xorAddr.GetFrom(msg); getErr != nil {
					log.Printf("failed to get XOR-MAPPED-ADDRESS: %s", getErr)
					continue
				}
				log.Printf("my address is: %v", xorAddr.String())
				network.PublicAddr = &net.UDPAddr{IP: xorAddr.IP, Port: xorAddr.Port}
				network.publicAddrCh <- network.PublicAddr
			} else {

				decodedMessage, err := Decode(data)
				if err != nil {
					log.Printf("failed to decode UDP packet from %v: %v\n", addr, err)
					continue
				}

				udpAddr, ok := addr.(*net.UDPAddr)
				if !ok {
					log.Printf("error converting the address to udp address")
					continue
				}

				select {
				case network.requestQueue <- UDPRequest{message: decodedMessage, address: udpAddr}:

				default:
					log.Printf("request queue is full, dropping package from %v", addr)
				}
			}
		}
	}()

	// read quic packets
	ln, err := tr.Listen(&tls.Config{InsecureSkipVerify: true, NextProtos: []string{"drone-net"}}, &quic.Config{})
	if err != nil {
		return fmt.Errorf("error listening quic: %v", err)
	}

	for {
		conn, err := ln.Accept(ctx)
		log.Println("yo I got a quic message")
		if err != nil {
			return err
		}
		go func() {
			packet, err := conn.AcceptStream(ctx)
			if err != nil {
				return
			}
			buf := make([]byte, MaxUDPPacketSize)
			n, _ := packet.Read(buf)

			data := buf[:n]

			log.Printf("data: %v", data)
		}()

		//		for {
		//
		//			buf := make([]byte, MaxUDPPacketSize)
		//			n, addr, err := network.conn.ReadFromUDP(buf)
		//			if err != nil {
		//				return fmt.Errorf("error reading from UDP: %v", err)
		//			}
		//			data := buf[:n]
		///
	}
}

func (network *QUICNetwork) requestHandlerWorker(ctx context.Context) error {
	for {
		request := <-network.requestQueue
		if network.rpcHandler == nil {
			log.Println("rpc handler is not yet set")
			continue
		}
		network.handleIncomingRequest(ctx, request.message, request.address)
	}
}

func (network *QUICNetwork) handleIncomingRequest(ctx context.Context, message *RpcMessage, addr *net.UDPAddr) {
	if ctx.Err() != nil {
		return
	}
	switch message.OpCode {
	case Ping:
		log.Printf("[RECV] Ping from=%s port=%d", truncateID(message.SelfNodeId), addr.Port)
		network.rpcHandler.HandlePing(ctx, Contact{ID: message.SelfNodeId, IP: addr.IP, Port: addr.Port})
		pingResponse := &RpcMessage{
			MessageID:      message.MessageID,
			OpCode:         Pong,
			SelfNodeId:     message.SelfNodeId,
			Key:            message.Key,
			ValueLength:    0,
			Value:          nil,
			ContactsLength: 0,
			Contacts:       nil,
		}
		encodedMessage, err := Encode(pingResponse)
		if err != nil {
			log.Printf("error encoding Pong response: %v\n", err)
			return
		}
		_, err = network.conn.WriteToUDP(encodedMessage, addr)
		if err != nil {
			log.Printf("error sending a response to addr: %v, : %v\n", addr, err)
			return
		}
		log.Printf("[SEND] Pong to=%s port=%d", truncateID(message.SelfNodeId), addr.Port)

	case FindNodeRequest:
		log.Printf("[RECV] FindNodeRequest from=%s port=%d key=%s", truncateID(message.SelfNodeId), addr.Port, truncateID(message.Key))
		contacts := network.rpcHandler.HandleFindNode(ctx, Contact{ID: message.SelfNodeId, IP: addr.IP, Port: addr.Port}, message.Key)
		// TODO: remove unnecessary fields like key, they don't need to be included in the response
		findNodeResponse := &RpcMessage{
			MessageID:      message.MessageID,
			OpCode:         FindNodeResponse,
			SelfNodeId:     message.SelfNodeId,
			Key:            message.Key,
			ValueLength:    0,
			Value:          nil,
			ContactsLength: uint64(len(contacts)),
			Contacts:       contacts,
		}
		encodedMessage, err := Encode(findNodeResponse)
		if err != nil {
			log.Printf("error encoding FindNodeResponse: %v\n", err)
			return
		}
		_, err = network.conn.WriteToUDP(encodedMessage, addr)
		if err != nil {
			log.Printf("error sending a response to addr: %v, : %v\n", addr, err)
			return
		}
		log.Printf("[SEND] FindNodeResponse to=%s port=%d ip=%v contacts=%s", truncateID(message.SelfNodeId), addr.Port, addr.IP, formatContacts(contacts))

	case FindValueRequest:
		log.Printf("[RECV] FindValueRequest from=%s port=%d key=%s valueLen=%d", truncateID(message.SelfNodeId), addr.Port, truncateID(message.Key), message.ValueLength)
		value, contacts := network.rpcHandler.HandleFindValue(ctx, Contact{ID: message.SelfNodeId, IP: addr.IP, Port: addr.Port}, message.Key)
		findValueResponse := &RpcMessage{
			MessageID:      message.MessageID,
			OpCode:         FindValueResponse,
			SelfNodeId:     message.SelfNodeId,
			Key:            message.Key,
			ValueLength:    uint64(len(value)),
			Value:          value,
			ContactsLength: uint64(len(contacts)),
			Contacts:       contacts,
		}
		encodedMessage, err := Encode(findValueResponse)
		if err != nil {
			log.Printf("error encoding FindValueResponse: %v\n", err)
			return
		}
		_, err = network.conn.WriteToUDP(encodedMessage, addr)
		if err != nil {
			log.Printf("error sending a response to addr: %v, : %v\n", addr, err)
			return
		}
		log.Printf("[SEND] FindValueResponse to=%s port=%d key=%s valueLen=%v", truncateID(message.SelfNodeId), addr.Port, truncateID(message.Key), findValueResponse.ValueLength)

	case StoreRequest:
		log.Printf("[RECV] StoreRequest from=%s port=%d key=%s valueLen=%d", truncateID(message.SelfNodeId), addr.Port, truncateID(message.Key), message.ValueLength)
		network.rpcHandler.HandleStore(ctx, Contact{ID: message.SelfNodeId, IP: addr.IP, Port: addr.Port}, message.Key, message.Value)
		storeResponse := &RpcMessage{
			MessageID:      message.MessageID,
			OpCode:         StoreResponse,
			SelfNodeId:     message.SelfNodeId,
			Key:            message.Key,
			ValueLength:    0,
			Value:          nil,
			ContactsLength: 0,
			Contacts:       nil,
		}
		encodedMessage, err := Encode(storeResponse)
		if err != nil {
			log.Printf("error encoding StoreResponse: %v\n", err)
			return
		}
		_, err = network.conn.WriteToUDP(encodedMessage, addr)
		if err != nil {
			log.Printf("error sending a response to addr: %v, : %v\n", addr, err)
			return
		}
		log.Printf("[SEND] StoreResponse to=%s port=%d key=%s", truncateID(message.SelfNodeId), addr.Port, truncateID(message.Key))

	case Pong:
		log.Printf("[RECV] Pong from=%s port=%d", truncateID(message.SelfNodeId), addr.Port)
		network.mu.Lock()
		responseChannel, exists := network.pending[message.MessageID]
		network.mu.Unlock()

		if !exists {
			log.Printf("request channel for Pong response is closed\n")
			return
		}
		responseChannel <- rpcResponse{} // no need for contacts since ping doesn't need them

	case StoreResponse:
		log.Printf("[RECV] StoreResponse from=%s port=%d key=%s", truncateID(message.SelfNodeId),
			addr.Port, truncateID(message.Key))
		network.mu.Lock()
		responseChannel, exists := network.pending[message.MessageID]
		network.mu.Unlock()

		if !exists {
			log.Printf("request channel for StoreResponse is closed\n")
			return
		}
		responseChannel <- rpcResponse{} // no need for contacts since store response doesn't need them

	case FindNodeResponse:
		log.Printf("[RECV] FindNodeResponse from=%s port=%d contacts=%s",
			truncateID(message.SelfNodeId), addr.Port, formatContacts(message.Contacts))
		network.mu.Lock()
		responseChannel, exists := network.pending[message.MessageID]
		network.mu.Unlock()

		if !exists {
			log.Printf("request channel FindNodeReponse is closed\n")
			return
		}
		responseChannel <- rpcResponse{Contacts: message.Contacts}

	case FindValueResponse:
		log.Printf("[RECV] FindValueResponse from=%s port=%d contacts=%s",
			truncateID(message.SelfNodeId), addr.Port, formatContacts(message.Contacts))
		network.mu.Lock()
		responseChannel, exists := network.pending[message.MessageID]
		network.mu.Unlock()
		if !exists {
			log.Printf("request channel for FindValueResponse is closed \n")
			return
		}
		responseChannel <- rpcResponse{Contacts: message.Contacts, Value: message.Value}

	default:
		break
	}
}

// FindNode implements Network.
func (network *QUICNetwork) FindNode(ctx context.Context, requester Contact, recipient Contact, targetID NodeId) ([]Contact, error) {
	ctx, cancel := context.WithTimeout(ctx, timeoutDuration*time.Millisecond)
	defer cancel()

	resultsChan := make(chan rpcResponse, 1)
	rpcMessage := &RpcMessage{
		MessageID:      NewRandomId(),
		OpCode:         FindNodeRequest,
		SelfNodeId:     requester.ID,
		Key:            targetID,
		ValueLength:    0,
		Value:          nil,
		ContactsLength: 0,
		Contacts:       nil,
	}
	network.mu.Lock()
	network.pending[rpcMessage.MessageID] = resultsChan
	network.mu.Unlock()

	defer func() {
		network.mu.Lock()
		delete(network.pending, rpcMessage.MessageID)
		network.mu.Unlock()
	}()
	log.Printf("[SEND] FindNodeRequest to=%s port=%v key=%s", truncateID(recipient.ID), recipient.Port, truncateID(targetID))
	msgToSend, err := Encode(rpcMessage)
	if err != nil {
		return nil, fmt.Errorf("error encoding message: %v", err)
	}
	_, err = network.conn.WriteToUDP(msgToSend, &net.UDPAddr{IP: recipient.IP, Port: recipient.Port})
	if err != nil {
		return nil, fmt.Errorf("error writing to udp: %v", err)
	}

	select {
	case result := <-resultsChan:
		var filteredContacts []Contact
		for _, contact := range result.Contacts {
			if contact.ID != requester.ID {
				filteredContacts = append(filteredContacts, contact)
			}
		}
		return filteredContacts, nil
	case <-ctx.Done():
		log.Printf("[TIMEOUT] FindNodeRequest to=%s port=%d key=%s", truncateID(recipient.ID), recipient.Port, truncateID(targetID))
		return nil, fmt.Errorf("timeout waiting for FindNode response")
	}
}

// FindValue implements Network.
func (network *QUICNetwork) FindValue(ctx context.Context, requester Contact, recipient Contact, key NodeId) ([]byte, []Contact, error) {
	ctx, cancel := context.WithTimeout(ctx, timeoutDuration*time.Millisecond)
	defer cancel()
	resultsChan := make(chan rpcResponse, 1)
	rpcMessage := &RpcMessage{
		MessageID:      NewRandomId(),
		OpCode:         FindValueRequest,
		SelfNodeId:     requester.ID,
		Key:            key,
		ValueLength:    0,
		Value:          nil,
		ContactsLength: 0,
		Contacts:       nil,
	}
	network.mu.Lock()
	network.pending[rpcMessage.MessageID] = resultsChan
	network.mu.Unlock()

	defer func() {
		network.mu.Lock()
		delete(network.pending, rpcMessage.MessageID)
		network.mu.Unlock()
	}()

	log.Printf("[SEND] FindValueRequest to=%s port=%v key=%s", truncateID(recipient.ID), recipient.Port, truncateID(key))
	msgToSend, err := Encode(rpcMessage)
	if err != nil {
		return nil, nil, fmt.Errorf("error encoding message: %v", err)
	}
	_, err = network.conn.WriteToUDP(msgToSend, &net.UDPAddr{IP: recipient.IP, Port: recipient.Port})
	if err != nil {
		return nil, nil, fmt.Errorf("error writing to udp: %v", err)
	}
	select {
	case result := <-resultsChan:
		if len(result.Value) != 0 {
			return result.Value, nil, nil
		}

		var filteredContacts []Contact
		for _, contact := range result.Contacts {
			if contact.ID != requester.ID {
				filteredContacts = append(filteredContacts, contact)
			}
		}
		return nil, filteredContacts, nil
	case <-ctx.Done():
		log.Printf("[TIMEOUT] FindValueRequest to=%s port=%d key=%s", truncateID(recipient.ID), recipient.Port, truncateID(key))
		return nil, nil, fmt.Errorf("timeout waiting for FindValue response")
	}
}

// Ping implements Network.
func (network *QUICNetwork) Ping(ctx context.Context, requester Contact, recipient Contact) error {
	ctx, cancel := context.WithTimeout(ctx, timeoutDuration*time.Millisecond)
	defer cancel()
	resultsChan := make(chan rpcResponse, 1)
	rpcMessage := &RpcMessage{
		MessageID:      NewRandomId(),
		OpCode:         Ping,
		SelfNodeId:     requester.ID,
		Key:            NodeId{},
		ValueLength:    0,
		Value:          nil,
		ContactsLength: 0,
		Contacts:       nil,
	}

	network.mu.Lock()
	network.pending[rpcMessage.MessageID] = resultsChan
	network.mu.Unlock()

	defer func() {
		network.mu.Lock()
		delete(network.pending, rpcMessage.MessageID)
		network.mu.Unlock()
	}()

	log.Printf("[SEND] Ping to=%s port=%v", truncateID(recipient.ID), recipient.Port)
	msgToSend, err := Encode(rpcMessage)
	if err != nil {
		return fmt.Errorf("error encoding message: %v", err)
	}
	_, err = network.conn.WriteToUDP(msgToSend, &net.UDPAddr{IP: recipient.IP, Port: recipient.Port})
	if err != nil {
		return fmt.Errorf("error writing to udp: %v", err)
	}

	select {
	case <-resultsChan:
		return nil
	case <-ctx.Done():
		log.Printf("[TIMEOUT] Ping to=%s port=%d", truncateID(recipient.ID), recipient.Port)
		return fmt.Errorf("timeout waiting for Pong")
	}
}

// Store implements Network.
func (network *QUICNetwork) Store(ctx context.Context, requester Contact, recipient Contact, key NodeId, value []byte) error {
	resultsChan := make(chan rpcResponse, 1)
	rpcMessage := &RpcMessage{
		MessageID:      NewRandomId(),
		OpCode:         StoreRequest,
		SelfNodeId:     requester.ID,
		Key:            key,
		ValueLength:    uint64(len(value)),
		Value:          value,
		ContactsLength: 0,
		Contacts:       nil,
	}
	network.mu.Lock()
	network.pending[rpcMessage.MessageID] = resultsChan
	network.mu.Unlock()

	defer func() {
		network.mu.Lock()
		delete(network.pending, rpcMessage.MessageID)
		network.mu.Unlock()
	}()

	log.Printf("[SEND] StoreRequest to=%s port=%d key=%s valueLen=%d", truncateID(recipient.ID), recipient.Port, truncateID(key), len(value))

	msgToSend, err := Encode(rpcMessage)
	if err != nil {
		return fmt.Errorf("error encoding message: %v", err)
	}
	_, err = network.conn.WriteToUDP(msgToSend, &net.UDPAddr{IP: recipient.IP, Port: recipient.Port})
	if err != nil {
		return fmt.Errorf("error writing to udp: %v", err)
	}

	select {
	case <-ctx.Done():
		log.Printf("[TIMEOUT] StoreRequest to=%s port=%d key=%s", truncateID(recipient.ID), recipient.Port, truncateID(key))
		return fmt.Errorf("request timed out")
	case <-resultsChan:
		return nil
	}
}
