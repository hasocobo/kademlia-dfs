package network

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/kademlia-dfs/kademlia-dfs/internal/dht"
	"github.com/kademlia-dfs/kademlia-dfs/internal/storage"
)

// MessageType represents the type of RPC message
type MessageType uint8

const (
	MessageTypePing      MessageType = 1
	MessageTypeFindNode  MessageType = 2
	MessageTypeFindValue MessageType = 3
	MessageTypeStore     MessageType = 4
)

// Message represents a DHT RPC message
type Message struct {
	Type      MessageType   `json:"type"`
	SenderID  string        `json:"sender_id"`
	SenderIP  string        `json:"sender_ip"`
	SenderPort int          `json:"sender_port"`
	TargetID  string        `json:"target_id,omitempty"`
	Value     []byte        `json:"value,omitempty"`
	Nodes     []*dht.Node   `json:"nodes,omitempty"`
	Data      []byte        `json:"data,omitempty"`
	RequestID uint64        `json:"request_id"`
}

// Response represents a response to a DHT RPC message
type Response struct {
	Type      MessageType   `json:"type"`
	Nodes     []*dht.Node   `json:"nodes,omitempty"`
	Value     []byte        `json:"value,omitempty"`
	Success   bool          `json:"success"`
	RequestID uint64        `json:"request_id"`
}

// Transport handles network communication
type Transport struct {
	mu          sync.RWMutex
	listener    net.Listener
	port        int
	localNode   *dht.Node
	dht         *dht.DHT
	contentStore ContentStore
	handler     MessageHandler
	connections map[string]net.Conn
	requestID   uint64
	pendingReqs map[uint64]chan *Response
}

// ContentStore interface for content storage
type ContentStore interface {
	HasFile(hash storage.ContentHash) bool
	GetFile(hash storage.ContentHash) ([]byte, error)
}

// MessageHandler handles incoming messages
type MessageHandler interface {
	HandlePing(msg *Message) *Response
	HandleFindNode(msg *Message) *Response
	HandleFindValue(msg *Message) *Response
	HandleStore(msg *Message) *Response
}

// NewTransport creates a new transport layer
func NewTransport(port int, localNode *dht.Node, dht *dht.DHT) *Transport {
	return &Transport{
		port:        port,
		localNode:   localNode,
		dht:         dht,
		connections: make(map[string]net.Conn),
		requestID:   0,
		pendingReqs: make(map[uint64]chan *Response),
	}
}

// Start starts the transport server
func (t *Transport) Start() error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", t.port))
	if err != nil {
		return fmt.Errorf("failed to start listener: %w", err)
	}

	t.mu.Lock()
	t.listener = listener
	t.mu.Unlock()

	go t.acceptConnections()
	return nil
}

// Stop stops the transport server
func (t *Transport) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.listener != nil {
		return t.listener.Close()
	}
	return nil
}

// acceptConnections accepts incoming connections
func (t *Transport) acceptConnections() {
	for {
		conn, err := t.listener.Accept()
		if err != nil {
			return
		}
		go t.handleConnection(conn)
	}
}

// handleConnection handles a single connection
func (t *Transport) handleConnection(conn net.Conn) {
	defer conn.Close()

	for {
		// Read message length
		var length uint32
		if err := binary.Read(conn, binary.BigEndian, &length); err != nil {
			return
		}

		// Read message data
		data := make([]byte, length)
		if _, err := conn.Read(data); err != nil {
			return
		}

		// Deserialize message
		var msg Message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		// Handle message
		var resp *Response
		switch msg.Type {
		case MessageTypePing:
			resp = t.handlePing(&msg)
		case MessageTypeFindNode:
			resp = t.handleFindNode(&msg)
		case MessageTypeFindValue:
			resp = t.handleFindValue(&msg)
		case MessageTypeStore:
			resp = t.handleStore(&msg)
		default:
			resp = &Response{Success: false, RequestID: msg.RequestID}
		}

		// Send response
		t.sendResponse(conn, resp)
	}
}

// handlePing handles ping messages
func (t *Transport) handlePing(msg *Message) *Response {
	// Update routing table
	nodeID, err := dht.NodeIDFromString(msg.SenderID)
	if err == nil {
		node := dht.NewNode(nodeID, msg.SenderIP, msg.SenderPort)
		if t.dht != nil {
			t.dht.GetRoutingTable().AddNode(node)
		}
	}

	return &Response{
		Type:      MessageTypePing,
		Success:   true,
		RequestID: msg.RequestID,
	}
}

// handleFindNode handles find node messages
func (t *Transport) handleFindNode(msg *Message) *Response {
	targetID, err := dht.NodeIDFromString(msg.TargetID)
	if err != nil {
		return &Response{Success: false, RequestID: msg.RequestID}
	}

	var nodes []*dht.Node
	if t.dht != nil {
		nodes = t.dht.GetRoutingTable().FindClosestNodes(targetID, dht.K)
	}

	// Update routing table with sender
	nodeID, err := dht.NodeIDFromString(msg.SenderID)
	if err == nil {
		node := dht.NewNode(nodeID, msg.SenderIP, msg.SenderPort)
		if t.dht != nil {
			t.dht.GetRoutingTable().AddNode(node)
		}
	}

	return &Response{
		Type:      MessageTypeFindNode,
		Nodes:     nodes,
		Success:   true,
		RequestID: msg.RequestID,
	}
}

// handleFindValue handles find value messages
func (t *Transport) handleFindValue(msg *Message) *Response {
	targetID, err := dht.NodeIDFromString(msg.TargetID)
	if err != nil {
		return &Response{Success: false, RequestID: msg.RequestID}
	}

	// Convert NodeID to ContentHash for lookup
	var hash storage.ContentHash
	copy(hash[:], targetID[:])

	// Check if we have the value
	if t.contentStore != nil && t.contentStore.HasFile(hash) {
		value, err := t.contentStore.GetFile(hash)
		if err == nil {
			return &Response{
				Type:      MessageTypeFindValue,
				Value:     value,
				Success:   true,
				RequestID: msg.RequestID,
			}
		}
	}

	// Return closest nodes
	var nodes []*dht.Node
	if t.dht != nil {
		nodes = t.dht.GetRoutingTable().FindClosestNodes(targetID, dht.K)
	}

	// Update routing table with sender
	nodeID, err := dht.NodeIDFromString(msg.SenderID)
	if err == nil {
		node := dht.NewNode(nodeID, msg.SenderIP, msg.SenderPort)
		if t.dht != nil {
			t.dht.GetRoutingTable().AddNode(node)
		}
	}

	return &Response{
		Type:      MessageTypeFindValue,
		Nodes:     nodes,
		Success:   true,
		RequestID: msg.RequestID,
	}
}

// handleStore handles store messages
func (t *Transport) handleStore(msg *Message) *Response {
	// Update routing table with sender
	nodeID, err := dht.NodeIDFromString(msg.SenderID)
	if err == nil {
		node := dht.NewNode(nodeID, msg.SenderIP, msg.SenderPort)
		if t.dht != nil {
			t.dht.GetRoutingTable().AddNode(node)
		}
	}

	// Store would be handled by storage layer
	return &Response{
		Type:      MessageTypeStore,
		Success:   true,
		RequestID: msg.RequestID,
	}
}

// sendResponse sends a response over the connection
func (t *Transport) sendResponse(conn net.Conn, resp *Response) {
	data, err := json.Marshal(resp)
	if err != nil {
		return
	}

	length := uint32(len(data))
	if err := binary.Write(conn, binary.BigEndian, length); err != nil {
		return
	}
	if _, err := conn.Write(data); err != nil {
		return
	}
}

// SendPing sends a ping message to a node
func (t *Transport) SendPing(node *dht.Node) error {
	return t.sendMessage(node, MessageTypePing, "", nil)
}

// SendFindNode sends a find node message
func (t *Transport) SendFindNode(node *dht.Node, target dht.NodeID) ([]*dht.Node, error) {
	resp, err := t.sendRequest(node, MessageTypeFindNode, target.String(), nil)
	if err != nil {
		return nil, err
	}
	return resp.Nodes, nil
}

// SendFindValue sends a find value message
func (t *Transport) SendFindValue(node *dht.Node, target dht.NodeID) ([]*dht.Node, []byte, error) {
	resp, err := t.sendRequest(node, MessageTypeFindValue, target.String(), nil)
	if err != nil {
		return nil, nil, err
	}
	if resp.Value != nil {
		return nil, resp.Value, nil
	}
	return resp.Nodes, nil, nil
}

// SendStore sends a store message
func (t *Transport) SendStore(node *dht.Node, key dht.NodeID, value []byte) error {
	return t.sendMessage(node, MessageTypeStore, key.String(), value)
}

// sendMessage sends a message without waiting for response
func (t *Transport) sendMessage(node *dht.Node, msgType MessageType, targetID string, data []byte) error {
	conn, err := net.DialTimeout("tcp", node.Address(), 5*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()

	msg := &Message{
		Type:      msgType,
		SenderID:  t.localNode.ID.String(),
		SenderIP:  t.localNode.IP,
		SenderPort: t.localNode.Port,
		TargetID:  targetID,
		Data:      data,
		RequestID: 0, // No response needed
	}

	return t.writeMessage(conn, msg)
}

// sendRequest sends a message and waits for response
func (t *Transport) sendRequest(node *dht.Node, msgType MessageType, targetID string, data []byte) (*Response, error) {
	conn, err := net.DialTimeout("tcp", node.Address(), 5*time.Second)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	t.mu.Lock()
	t.requestID++
	reqID := t.requestID
	respChan := make(chan *Response, 1)
	t.pendingReqs[reqID] = respChan
	t.mu.Unlock()

	msg := &Message{
		Type:      msgType,
		SenderID:  t.localNode.ID.String(),
		SenderIP:  t.localNode.IP,
		SenderPort: t.localNode.Port,
		TargetID:  targetID,
		Data:      data,
		RequestID: reqID,
	}

	if err := t.writeMessage(conn, msg); err != nil {
		t.mu.Lock()
		delete(t.pendingReqs, reqID)
		t.mu.Unlock()
		return nil, err
	}

	// Read response
	var length uint32
	if err := binary.Read(conn, binary.BigEndian, &length); err != nil {
		return nil, err
	}

	respData := make([]byte, length)
	if _, err := conn.Read(respData); err != nil {
		return nil, err
	}

	var resp Response
	if err := json.Unmarshal(respData, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// writeMessage writes a message to the connection
func (t *Transport) writeMessage(conn net.Conn, msg *Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	length := uint32(len(data))
	if err := binary.Write(conn, binary.BigEndian, length); err != nil {
		return err
	}
	if _, err := conn.Write(data); err != nil {
		return err
	}

	return nil
}

// SetContentStore sets the content store
func (t *Transport) SetContentStore(store ContentStore) {
	t.contentStore = store
}

