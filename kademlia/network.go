package kademliadfs

import (
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
)

type Network interface {
	Ping(requester, recipient Contact) error
	FindNode(requester, recipient Contact, targetID NodeId) ([]Contact, error)
	FindValue(requester, recipient Contact, key NodeId) ([]byte, []Contact, error)
	Store(requester, recipient Contact, key NodeId, value []byte) error
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
		return nil, fmt.Errorf("Error creating UDP listener: %v", err)
	}

	log.Printf("Listening on %v\n", udpConn.LocalAddr().String())

	udpNetwork := &UDPNetwork{
		conn:         udpConn,
		pending:      make(map[NodeId]chan rpcResponse),
		requestQueue: make(chan UDPRequest, packetBufferLimit),
	}

	// create n=workerPoolSize workers
	for i := 0; i < workerPoolSize; i++ {
		go udpNetwork.requestHandlerWorker()
	}

	return udpNetwork, nil
}

// FindNode implements Network.
func (network *UDPNetwork) FindNode(requester Contact, recipient Contact, targetID NodeId) ([]Contact, error) {
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
	msgToSend := network.Encode(rpcMessage)
	_, err := network.conn.WriteToUDP(msgToSend, &net.UDPAddr{IP: recipient.IP, Port: recipient.Port})
	if err != nil {
		return nil, fmt.Errorf("error writing to udp: %v", err)
	}
	// Times out if after 5000 ms
	select {
	case result := <-resultsChan:
		var filteredContacts []Contact
		for _, contact := range result.Contacts {
			if contact.ID != requester.ID {
				filteredContacts = append(filteredContacts, contact)
			}
		}
		return filteredContacts, nil
	case <-time.After(time.Millisecond * 5000):
		log.Printf("[TIMEOUT] FindNodeRequest to=%s port=%d key=%s", truncateID(recipient.ID), recipient.Port, truncateID(targetID))
		return nil, fmt.Errorf("timeout waiting for FindNode response")
	}
}

// FindValue implements Network.
func (network *UDPNetwork) FindValue(requester Contact, recipient Contact, key NodeId) ([]byte, []Contact, error) {
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
	msgToSend := network.Encode(rpcMessage)
	_, err := network.conn.WriteToUDP(msgToSend, &net.UDPAddr{IP: recipient.IP, Port: recipient.Port})
	if err != nil {
		return nil, nil, fmt.Errorf("error writing to udp: %v", err)
	}
	// Times out if after 5000 ms
	select {
	case result := <-resultsChan:
		if result.Value != nil {
			return result.Value, nil, nil
		}

		var filteredContacts []Contact
		for _, contact := range result.Contacts {
			if contact.ID != requester.ID {
				filteredContacts = append(filteredContacts, contact)
			}
		}
		return nil, filteredContacts, nil
	case <-time.After(time.Millisecond * 5000):
		log.Printf("[TIMEOUT] FindValueRequest to=%s port=%d key=%s", truncateID(recipient.ID), recipient.Port, truncateID(key))
		return nil, nil, fmt.Errorf("timeout waiting for FindValue response")
	}
}

// Ping implements Network.
func (network *UDPNetwork) Ping(requester Contact, recipient Contact) error {
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
	msgToSend := network.Encode(rpcMessage)
	_, err := network.conn.WriteToUDP(msgToSend, &net.UDPAddr{IP: recipient.IP, Port: recipient.Port})
	if err != nil {
		return fmt.Errorf("error writing to udp: %v", err)
	}

	select {
	case <-resultsChan:
		return nil
	case <-time.After(time.Millisecond * 5000):
		log.Printf("[TIMEOUT] Ping to=%s port=%d", truncateID(recipient.ID), recipient.Port)
		return fmt.Errorf("timeout waiting for Pong")
	}
}

// Store implements Network.
func (network *UDPNetwork) Store(requester Contact, recipient Contact, key NodeId, value []byte) error {
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

	msgToSend := network.Encode(rpcMessage)
	_, err := network.conn.WriteToUDP(msgToSend, &net.UDPAddr{IP: recipient.IP, Port: recipient.Port})
	if err != nil {
		return fmt.Errorf("error writing to udp: %v", err)
	}

	select {
	case <-time.After(time.Second * 5):
		log.Printf("[TIMEOUT] StoreRequest to=%s port=%d key=%s", truncateID(recipient.ID), recipient.Port, truncateID(key))
		return fmt.Errorf("request timed out: %v", err)
	case <-resultsChan:
		return nil
	}
}

type RpcHandler interface {
	HandlePing(requester Contact) bool
	HandleFindNode(requester Contact, key NodeId) []Contact
	HandleFindValue(requester Contact, key NodeId) ([]byte, []Contact)
	HandleStore(requester Contact, key NodeId, value []byte)
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

// TODO: remove replace log.Fatalf with printf and return []byte, error
// Encode takes RpcMessage struct and converts it to raw UDP bytes
func (network *UDPNetwork) Encode(message *RpcMessage) []byte {
	encodedMessage := make([]byte, 0)
	var err error

	encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, message.MessageID)
	if err != nil {
		log.Fatalf("failed encoding messageId: %v", err)
	}

	encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, message.OpCode)
	if err != nil {
		log.Fatalf("failed encoding opCode: %v", err)
	}

	encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, message.SelfNodeId)
	if err != nil {
		log.Fatalf("failed encoding NodeId: %v", err)
	}

	encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, message.Key)
	if err != nil {
		log.Fatalf("failed encoding Key: %v", err)
	}

	encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, message.ValueLength)
	if err != nil {
		log.Fatalf("failed encoding value length: %v", err)
	}

	encodedMessage = append(encodedMessage, message.Value...)

	encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, message.ContactsLength)
	if err != nil {
		log.Fatalf("failed encoding contacts length: %v", err)
	}

	for _, contact := range message.Contacts {
		encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, contact.ID)
		if err != nil {
			log.Fatalf("failed encoding contact Id: %v", err)
		}

		encodedMessage = append(encodedMessage, contact.IP.To4()...)

		encodedMessage = binary.BigEndian.AppendUint16(encodedMessage, uint16(contact.Port))
	}

	return encodedMessage
}

func (network *UDPNetwork) requestHandlerWorker() {
	for {
		select {
		case request := <-network.requestQueue:
			network.handleIncomingRequest(request.message, request.address)
		}
		// TODO: add context.timeout
	}
}

func (network *UDPNetwork) handleIncomingRequest(message *RpcMessage, addr *net.UDPAddr) {
	switch message.OpCode {
	case Ping:
		log.Printf("[RECV] Ping from=%s port=%d", truncateID(message.SelfNodeId), addr.Port)
		network.rpcHandler.HandlePing(Contact{ID: message.SelfNodeId, IP: addr.IP, Port: addr.Port})
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
		_, err := network.conn.WriteToUDP(network.Encode(pingResponse), addr)
		if err != nil {
			log.Printf("error sending a response to addr: %v, : %v\n", addr, err)
			return
		}
		log.Printf("[SEND] Pong to=%s port=%d", truncateID(message.SelfNodeId), addr.Port)

	case FindNodeRequest:
		log.Printf("[RECV] FindNodeRequest from=%s port=%d key=%s", truncateID(message.SelfNodeId), addr.Port, truncateID(message.Key))
		contacts := network.rpcHandler.HandleFindNode(Contact{ID: message.SelfNodeId, IP: addr.IP, Port: addr.Port}, message.Key)
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
		_, err := network.conn.WriteToUDP(network.Encode(findNodeResponse), addr)
		if err != nil {
			log.Printf("error sending a response to addr: %v, : %v\n", addr, err)
			return
		}
		log.Printf("[SEND] FindNodeResponse to=%s port=%d ip=%v contacts=%s", truncateID(message.SelfNodeId), addr.Port, addr.IP, formatContacts(contacts))

	case FindValueRequest:
		log.Printf("[RECV] FindValueRequest from=%s port=%d key=%s valueLen=%d", truncateID(message.SelfNodeId), addr.Port, truncateID(message.Key), message.ValueLength)
		value, contacts := network.rpcHandler.HandleFindValue(Contact{ID: message.SelfNodeId, IP: addr.IP, Port: addr.Port}, message.Key)
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
		_, err := network.conn.WriteToUDP(network.Encode(findValueResponse), addr)
		if err != nil {
			log.Printf("error sending a response to addr: %v, : %v\n", addr, err)
			return
		}
		log.Printf("[SEND] FindValueResponse to=%s port=%d key=%s valueLen=%v", truncateID(message.SelfNodeId), addr.Port, truncateID(message.Key), findValueResponse.ValueLength)

	case StoreRequest:
		log.Printf("[RECV] StoreRequest from=%s port=%d key=%s valueLen=%d", truncateID(message.SelfNodeId), addr.Port, truncateID(message.Key), message.ValueLength)
		network.rpcHandler.HandleStore(Contact{ID: message.SelfNodeId, IP: addr.IP, Port: addr.Port}, message.Key, message.Value)
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
		_, err := network.conn.WriteToUDP(network.Encode(storeResponse), addr)
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
			fmt.Printf("request channel for Pong response is closed\n")
			return
		}
		responseChannel <- rpcResponse{} // no need for contacts since ping doesn't need them

	case StoreResponse:
		log.Printf("[RECV] StoreResponse from=%s port=%d key=%s", truncateID(message.SelfNodeId), addr.Port, truncateID(message.Key))
		network.mu.Lock()
		responseChannel, exists := network.pending[message.MessageID]
		network.mu.Unlock()

		if !exists {
			fmt.Printf("request channel for StoreResponse is closed\n")
			return
		}
		responseChannel <- rpcResponse{} // no need for contacts since store response doesn't need them

	case FindNodeResponse:
		log.Printf("[RECV] FindNodeResponse from=%s port=%d contacts=%s", truncateID(message.SelfNodeId), addr.Port, formatContacts(message.Contacts))
		network.mu.Lock()
		responseChannel, exists := network.pending[message.MessageID]
		network.mu.Unlock()

		if !exists {
			fmt.Printf("request channel FindNodeReponse is closed\n")
			return
		}
		responseChannel <- rpcResponse{Contacts: message.Contacts}

	case FindValueResponse:
		log.Printf("[RECV] FindValueResponse from=%s port=%d contacts=%s", truncateID(message.SelfNodeId), addr.Port, formatContacts(message.Contacts))
		network.mu.Lock()
		responseChannel, exists := network.pending[message.MessageID]
		network.mu.Unlock()
		if !exists {
			fmt.Printf("request channel for FindValueResponse is closed \n")
			return
		}
		responseChannel <- rpcResponse{Contacts: message.Contacts, Value: message.Value}

	}
}

func (network *UDPNetwork) Listen() error {
	defer network.conn.Close()
	for {
		buf := make([]byte, MaxUDPPacketSize)
		n, addr, err := network.conn.ReadFromUDP(buf)
		if err != nil {
			log.Fatalf("error reading from UDP: %v", err)
		}
		data := buf[:n]
		decodedMessage, err := network.Decode(data)
		if err != nil {
			log.Printf("failed to decode UDP packet from %v: %v\n", addr, err)
			continue
		}

		network.requestQueue <- UDPRequest{message: decodedMessage, address: addr}
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

// Simulation network implements Network to test the core RPC calls without the complexity of UDP yet
type SimNetwork struct {
	Nodes map[NodeId]*Node
	mu    sync.Mutex
}

func NewSimNetwork() *SimNetwork {
	return &SimNetwork{
		Nodes: make(map[NodeId]*Node),
	}
}

func (sn *SimNetwork) Ping(requester, recipient Contact) error {
	sn.mu.Lock()
	defer sn.mu.Unlock()

	if node, exists := sn.Nodes[recipient.ID]; exists {
		node.HandlePing(requester)
	}

	return nil
}

func (sn *SimNetwork) FindNode(requester, recipient Contact, targetID NodeId) ([]Contact, error) {
	sn.mu.Lock()
	defer sn.mu.Unlock()

	recipientNode, exists := sn.Nodes[recipient.ID]
	if !exists {
		return nil, fmt.Errorf("recipient node %v not found", recipient.ID)
	}

	return recipientNode.HandleFindNode(requester, targetID), nil
}

func (sn *SimNetwork) FindValue(requester, recipient Contact, key NodeId) ([]byte, []Contact, error) {
	sn.mu.Lock()
	defer sn.mu.Unlock()

	recipientNode, exists := sn.Nodes[recipient.ID]
	if !exists {
		return nil, nil, fmt.Errorf("recipient node %v not found", recipient.ID)
	}

	value, contacts := recipientNode.HandleFindValue(requester, key)
	return value, contacts, nil
}

func (sn *SimNetwork) Store(requester, recipient Contact, key NodeId, value []byte) error {
	sn.mu.Lock()
	defer sn.mu.Unlock()

	recipientNode, exists := sn.Nodes[recipient.ID]
	if !exists {
		return fmt.Errorf("recipient node %v not found", recipient.ID)
	}

	recipientNode.HandleStore(requester, key, value)
	return nil
}

func (sn *SimNetwork) Register(n *Node) {
	sn.mu.Lock()
	defer sn.mu.Unlock()

	sn.Nodes[n.Self.ID] = n
}
