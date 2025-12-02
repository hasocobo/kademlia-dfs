package kademliadfs

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"
)

const (
	MaxUDPPacketSize = 65535
)

type Network interface {
	Ping(requster, recipient Contact) error
	FindNode(requester, recipient Contact, targetID NodeId) ([]Contact, error)
	FindValue(requester, recipient Contact, key NodeId) ([]byte, []Contact, error)
	Store(requester, recipient Contact, key NodeId, value []byte) error
}

type UDPNetwork struct {
	conn              *net.UDPConn
	remoteConnections sync.Map
	rpcHandler        RpcHandler

	mu sync.Mutex
}

// FindNode implements Network.
func (network *UDPNetwork) FindNode(requester Contact, recipient Contact, targetID NodeId) ([]Contact, error) {
	panic("unimplemented")
}

// FindValue implements Network.
func (network *UDPNetwork) FindValue(requester Contact, recipient Contact, key NodeId) ([]byte, []Contact, error) {
	panic("unimplemented")
}

// Ping implements Network.
func (network *UDPNetwork) Ping(requster Contact, recipient Contact) error {
	panic("unimplemented")
}

// Store implements Network.
func (network *UDPNetwork) Store(requester Contact, recipient Contact, key NodeId, value []byte) error {
	panic("unimplemented")
}

type RpcHandler interface {
	HandlePing(requester Contact) bool
	HandleFindNode(requester Contact, key NodeId) []Contact
	HandleFindValue(requester Contact, key NodeId) ([]byte, []Contact)
	HandleStore(requester Contact, key NodeId, value []byte)
}

// Fixed length Rpc Message structure for now, might migrate to protobuf/avro in the future
type RpcMessage struct {
	OpCode         OpCode // Rpc Message Type is decided based on this value
	SelfNodeId     NodeId
	Key            NodeId
	ValueLength    int
	Value          []byte
	ContactsLength int
	Contacts       []Contact
}

type OpCode int

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

func NewUDPNetwork(ip net.IP, port int) *UDPNetwork {
	udpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: ip, Port: port})
	if err != nil {
		panic(fmt.Sprintf("Error creating UDP listener: %v", err))
	}

	fmt.Printf("Listening on %v", udpConn.LocalAddr().String())
	fmt.Println()

	return &UDPNetwork{conn: udpConn, remoteConnections: *new(sync.Map)}
}

func (network *UDPNetwork) SetHandler(rpcHandler RpcHandler) {
	network.rpcHandler = rpcHandler
}

// Converts raw UDP packet bytes to RpcMessage struct
func (network *UDPNetwork) Decode(packet []byte) (*RpcMessage, error) {
	if len(packet) == 0 {
		return nil, fmt.Errorf("packet length is invalid: %d", len(packet))
	}
	opCode := int(binary.BigEndian.Uint64(packet[:1])) // Convert byte to int
	packet = packet[1:]

	selfNodeId := packet[:idLength]
	packet = packet[idLength:]

	key := packet[:idLength]
	packet = packet[idLength:]

	valueLength := int(binary.BigEndian.Uint64(packet[:1]))
	packet = packet[1:]

	value := packet[:valueLength]
	packet = packet[valueLength:]

	contactsLen := int(binary.BigEndian.Uint64(packet[:1]))
	packet = packet[1:]

	contacts := make([]Contact, contactsLen)
	for i := range contactsLen {
		contactId := packet[:idLength]
		packet = packet[idLength:]

		contactIp := packet[:net.IPv4len]
		packet = packet[net.IPv4len:]

		contactPort := packet[:16] // uint16
		packet = packet[16:]

		contacts[i] = Contact{ID: NodeId(contactId), IP: contactIp, Port: int(binary.BigEndian.Uint16(contactPort))}
	}

	return &RpcMessage{OpCode: OpCode(opCode),
			SelfNodeId:     NodeId(selfNodeId),
			Key:            NodeId(key),
			ValueLength:    valueLength,
			Value:          value,
			ContactsLength: contactsLen,
			Contacts:       contacts,
		},
		nil
}

// TAkes RpcMessage struct and converts it to raw UDP bytes
func (network *UDPNetwork) Encode(message RpcMessage) []byte {
	var encodedMessage []byte
	var err error

	encodedMessage = binary.BigEndian.AppendUint64(encodedMessage, uint64(message.OpCode))

	encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, message.SelfNodeId)
	if err != nil {
		return nil
	}

	encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, message.Key)
	if err != nil {
		return nil
	}

	encodedMessage = binary.BigEndian.AppendUint64(encodedMessage, uint64(message.ValueLength))

	encodedMessage = append(encodedMessage, message.Value...)

	encodedMessage = binary.BigEndian.AppendUint64(encodedMessage, uint64(message.ContactsLength))

	for _, contact := range message.Contacts {
		encodedMessage, err = binary.Append(encodedMessage, binary.BigEndian, contact.ID)
		if err != nil {
			return nil
		}

		encodedMessage = append(encodedMessage, contact.IP.To4()...)

		encodedMessage = binary.BigEndian.AppendUint16(encodedMessage, uint16(contact.Port))
	}

	return encodedMessage
}

func (network *UDPNetwork) Listen() error {
	defer network.conn.Close()
	for {
		buf := make([]byte, MaxUDPPacketSize)
		n, addr, err := network.conn.ReadFromUDP(buf)

		if err != nil {
			return fmt.Errorf("error reading from UDP: %v", err)
		}
		if _, exists := network.remoteConnections.Load(addr.String()); !exists {
			network.remoteConnections.Store(addr.String(), &addr)
		}

		data := buf[:n]

		message, err := network.Decode(data)
		if err != nil {
			fmt.Printf("failed to decode UDP packet from %v: %v\n", addr, err)
			continue
		}

		switch message.OpCode {
		case Ping:
			network.rpcHandler.HandlePing(Contact{ID: message.SelfNodeId, IP: addr.IP, Port: addr.Port})
		case FindNodeRequest:
			network.rpcHandler.HandleFindNode(Contact{ID: message.SelfNodeId, IP: addr.IP, Port: addr.Port}, message.Key)
		case FindValueRequest:
			network.rpcHandler.HandleFindValue(Contact{ID: message.SelfNodeId, IP: addr.IP, Port: addr.Port}, message.Key)
		case StoreRequest:
			network.rpcHandler.HandleStore(Contact{ID: message.SelfNodeId, IP: addr.IP, Port: addr.Port}, message.Key, message.Value)
		}
	}
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
