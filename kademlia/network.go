package kademliadfs

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

const (
	MaxUDPPacketSize  = 65535
	packetBufferLimit = 1024 // how many packets can wait in buffer
	workerPoolSize    = 32
	timeoutDuration   = 500 // in ms
)

type Network interface {
	Ping(ctx context.Context, requester, recipient Contact) error
	FindNode(ctx context.Context, requester, recipient Contact, targetID NodeId) ([]Contact, error)
	FindValue(ctx context.Context, requester, recipient Contact, key NodeId) ([]byte, []Contact, error)
	Store(ctx context.Context, equester, recipient Contact, key NodeId, value []byte) error
}

type UDPNetwork struct {
	conn       *net.UDPConn
	pending    map[NodeId]chan rpcResponse
	rpcHandler RpcHandler

	requestQueue chan UDPRequest
	mu           sync.Mutex
}

type UDPRequest struct {
	message *RpcMessage
	address *net.UDPAddr
}

type rpcResponse struct {
	Contacts []Contact
	Value    []byte
}

func NewUDPNetwork(ip net.IP, port int) (*UDPNetwork, error) {
	udpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: ip, Port: port})
	if err != nil {
		return nil, fmt.Errorf("error creating UDP listener: %v", err)
	}

	log.Printf("Listening on %v\n", udpConn.LocalAddr().String())

	udpNetwork := &UDPNetwork{
		conn:         udpConn,
		pending:      make(map[NodeId]chan rpcResponse),
		requestQueue: make(chan UDPRequest, packetBufferLimit),
	}

	// create n=workerPoolSize workers with timeout
	for range workerPoolSize {
		go udpNetwork.requestHandlerWorker(context.Background())
	}

	return udpNetwork, nil
}

// FindNode implements Network.
func (network *UDPNetwork) FindNode(ctx context.Context, requester Contact, recipient Contact, targetID NodeId) ([]Contact, error) {
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
	msgToSend, err := network.Encode(rpcMessage)
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
func (network *UDPNetwork) FindValue(ctx context.Context, requester Contact, recipient Contact, key NodeId) ([]byte, []Contact, error) {
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
	msgToSend, err := network.Encode(rpcMessage)
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
func (network *UDPNetwork) Ping(ctx context.Context, requester Contact, recipient Contact) error {
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
	msgToSend, err := network.Encode(rpcMessage)
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
func (network *UDPNetwork) Store(ctx context.Context, requester Contact, recipient Contact, key NodeId, value []byte) error {
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

	msgToSend, err := network.Encode(rpcMessage)
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

type RpcHandler interface {
	HandlePing(ctx context.Context, requester Contact) bool
	HandleFindNode(ctx context.Context, requester Contact, key NodeId) []Contact
	HandleFindValue(ctx context.Context, requester Contact, key NodeId) ([]byte, []Contact)
	HandleStore(ctx context.Context, requester Contact, key NodeId, value []byte)
}

// Fixed length Rpc Message structure for now, might migrate to protobuf/avro in the future
type RpcMessage struct {
	MessageID      NodeId
	OpCode         OpCode // Rpc Message Type is decided based on this value
	SelfNodeId     NodeId
	Key            NodeId
	ValueLength    uint64
	Value          []byte
	ContactsLength uint64
	Contacts       []Contact
}

type OpCode uint8

const (
	Ping OpCode = iota
	Pong
	FindNodeRequest
	FindNodeResponse
	FindValueRequest
	FindValueResponse
	StoreRequest
	StoreResponse
)

func (network *UDPNetwork) SetHandler(rpcHandler RpcHandler) {
	network.rpcHandler = rpcHandler
}

// Decode converts raw UDP packet bytes to RpcMessage struct
func (network *UDPNetwork) Decode(packet []byte) (*RpcMessage, error) {
	if len(packet) == 0 {
		return nil, fmt.Errorf("packet length is invalid: %d", len(packet))
	}
	messageId := packet[:idLength]
	packet = packet[idLength:]

	opCode := int(packet[0]) // Convert byte to int
	packet = packet[1:]

	selfNodeId := packet[:idLength]
	packet = packet[idLength:]

	key := packet[:idLength]
	packet = packet[idLength:]

	valueLength := binary.BigEndian.Uint64(packet[:8])
	packet = packet[8:]

	value := packet[:valueLength]
	packet = packet[valueLength:]

	contactsLen := binary.BigEndian.Uint64(packet[:8])
	packet = packet[8:]

	contacts := make([]Contact, contactsLen)
	for i := range contactsLen {
		contactId := packet[:idLength]
		packet = packet[idLength:]

		contactIp := packet[:net.IPv4len]
		packet = packet[net.IPv4len:]

		contactPort := packet[:2] // uint16
		packet = packet[2:]

		contacts[i] = Contact{ID: NodeId(contactId), IP: contactIp, Port: int(binary.BigEndian.Uint16(contactPort))}
	}

	return &RpcMessage{
			MessageID:      NodeId(messageId),
			OpCode:         OpCode(opCode),
			SelfNodeId:     NodeId(selfNodeId),
			Key:            NodeId(key),
			ValueLength:    valueLength,
			Value:          value,
			ContactsLength: contactsLen,
			Contacts:       contacts,
		},
		nil
}

// Encode takes RpcMessage struct and converts it to raw UDP bytes
func (network *UDPNetwork) Encode(message *RpcMessage) ([]byte, error) {
	encodedMessage := make([]byte, 0)
	var err error

	encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, message.MessageID)
	if err != nil {
		return nil, fmt.Errorf("failed encoding messageId: %v", err)
	}

	encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, message.OpCode)
	if err != nil {
		return nil, fmt.Errorf("failed encoding opCode: %v", err)
	}

	encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, message.SelfNodeId)
	if err != nil {
		return nil, fmt.Errorf("failed encoding NodeId: %v", err)
	}

	encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, message.Key)
	if err != nil {
		return nil, fmt.Errorf("failed encoding Key: %v", err)
	}

	encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, message.ValueLength)
	if err != nil {
		return nil, fmt.Errorf("failed encoding value length: %v", err)
	}

	encodedMessage = append(encodedMessage, message.Value...)

	encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, message.ContactsLength)
	if err != nil {
		return nil, fmt.Errorf("failed encoding contacts length: %v", err)
	}

	for _, contact := range message.Contacts {
		encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, contact.ID)
		if err != nil {
			return nil, fmt.Errorf("failed encoding contact Id: %v", err)
		}

		encodedMessage = append(encodedMessage, contact.IP.To4()...)

		encodedMessage = binary.BigEndian.AppendUint16(encodedMessage, uint16(contact.Port))
	}

	return encodedMessage, nil
}

func (network *UDPNetwork) requestHandlerWorker(ctx context.Context) error {
	for {
		request := <-network.requestQueue
		network.handleIncomingRequest(ctx, request.message, request.address)
	}
}

func (network *UDPNetwork) handleIncomingRequest(ctx context.Context, message *RpcMessage, addr *net.UDPAddr) {
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
		encodedMessage, err := network.Encode(pingResponse)
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
		encodedMessage, err := network.Encode(findNodeResponse)
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
		encodedMessage, err := network.Encode(findValueResponse)
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
		encodedMessage, err := network.Encode(storeResponse)
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
		log.Printf("[RECV] StoreResponse from=%s port=%d key=%s", truncateID(message.SelfNodeId), addr.Port, truncateID(message.Key))
		network.mu.Lock()
		responseChannel, exists := network.pending[message.MessageID]
		network.mu.Unlock()

		if !exists {
			log.Printf("request channel for StoreResponse is closed\n")
			return
		}
		responseChannel <- rpcResponse{} // no need for contacts since store response doesn't need them

	case FindNodeResponse:
		log.Printf("[RECV] FindNodeResponse from=%s port=%d contacts=%s", truncateID(message.SelfNodeId), addr.Port, formatContacts(message.Contacts))
		network.mu.Lock()
		responseChannel, exists := network.pending[message.MessageID]
		network.mu.Unlock()

		if !exists {
			log.Printf("request channel FindNodeReponse is closed\n")
			return
		}
		responseChannel <- rpcResponse{Contacts: message.Contacts}

	case FindValueResponse:
		log.Printf("[RECV] FindValueResponse from=%s port=%d contacts=%s", truncateID(message.SelfNodeId), addr.Port, formatContacts(message.Contacts))
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

func (network *UDPNetwork) Listen() error {
	defer network.conn.Close()
	for {
		buf := make([]byte, MaxUDPPacketSize)
		n, addr, err := network.conn.ReadFromUDP(buf)
		if err != nil {
			return fmt.Errorf("error reading from UDP: %v", err)
		}
		data := buf[:n]
		decodedMessage, err := network.Decode(data)
		if err != nil {
			log.Printf("failed to decode UDP packet from %v: %v\n", addr, err)
			continue
		}

		select {
		case network.requestQueue <- UDPRequest{message: decodedMessage, address: addr}:

		default:
			log.Printf("request queue is full, dropping package from %v", addr)
		}
	}
}

// Helper to truncate NodeId for logging
func truncateID(id NodeId) string {
	s := id.String()
	if len(s) > 8 {
		return s[:8] + "..."
	}
	return s
}

// Helper to format contacts for logging
func formatContacts(contacts []Contact) string {
	if len(contacts) == 0 {
		return "[]"
	}
	result := "["
	for i, c := range contacts {
		if i > 0 {
			result += ", "
		}
		result += truncateID(c.ID)
	}
	return result + "]"
}
