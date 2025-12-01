package kademliadfs

import (
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

	mu sync.Mutex
}

func NewUDPNetwork(ip net.IP, port int) *UDPNetwork {
	udpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: ip, Port: port})
	if err != nil {
		panic(fmt.Sprintf("Error creating UDP listener: %v", err))
	}

	fmt.Printf("Listening on %v", udpConn.LocalAddr().String())
	fmt.Println()

	return &UDPNetwork{conn: udpConn, remoteConnections: *new(sync.Map)}
}

func (n *UDPNetwork) Listen() {
	defer n.conn.Close()
	for {
		buf := make([]byte, MaxUDPPacketSize)
		_, addr, err := n.conn.ReadFromUDP(buf)

		if err != nil {
			fmt.Errorf("Error reading from UDP: %v", err)
		}

		if _, exists := n.remoteConnections.Load(addr.String()); !exists {
			n.remoteConnections.Store(addr.String(), &addr)
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
