package kademliadfs

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"sort"
	"sync"
)

type Node struct {
	Self         Contact
	RoutingTable *RoutingTable
	Network      Network
	Storage      map[NodeId][]byte

	mu sync.RWMutex
}

func NewNode(id NodeId, ip net.IP, port int, network Network) *Node {
	contact := Contact{ID: id, IP: ip, Port: port}
	rt := NewRoutingTable(id, func(c Contact) bool { return true })
	storage := make(map[NodeId][]byte)

	return &Node{Self: contact, RoutingTable: rt, Storage: storage, Network: network}
}

type NodeId [idLength]byte

func NewNodeId(name string) NodeId {
	return sha256.Sum256([]byte(name))
}

func (nodeId NodeId) String() string {
	return hex.EncodeToString(nodeId[:])
}

func (node *Node) HandlePing(requester Contact) bool {
	node.RoutingTable.Update(requester)

	return true
}

// This is the handler. Remote Producer Call(RPC) will call this function with its own Contact information.
// e.g with a 3 parameter function:
// FindNode(requester: Contact, recipientNodeToAsk: Node, targetId: NodeId) and in turn this
// will be handled by the function below by the recipientNodeToAsk.
func (node *Node) HandleFindNode(requester Contact, targetID NodeId) []Contact {
	node.RoutingTable.Update(requester)

	return node.RoutingTable.FindClosest(targetID, k)
}

// Return the value if it exists, return closest nodes if not
func (node *Node) HandleFindValue(requester Contact, key NodeId) ([]byte, []Contact) {
	node.RoutingTable.Update(requester)

	node.mu.RLock()
	defer node.mu.RUnlock()
	if value, ok := node.Storage[key]; ok {
		return value, nil
	}

	closestContacts := node.RoutingTable.FindClosest(key, k)
	return nil, closestContacts
}

func (node *Node) HandleStore(requester Contact, key NodeId, value []byte) {
	node.RoutingTable.Update(requester)

	node.mu.Lock()
	defer node.mu.RUnlock()

	node.Storage[key] = value
}

func (node *Node) Lookup(targetID NodeId) []Contact {
	// Grabs a few nodes from its own buckets for initial population
	shortlist := &ContactSorter{TargetID: targetID}
	shortlist.Contacts = append(shortlist.Contacts, node.RoutingTable.FindClosest(targetID, alphaConcurrency)...)

	if len(shortlist.Contacts) == 0 {
		fmt.Println("Found no contacts in buckets")
		return nil
	}

	sort.Sort(shortlist)

	queried := make(map[NodeId]bool, k)
	snapshot := shortlist.Contacts[:]

	foundCloserNode := true
	// Then queries these nodes and keeps populating nodes to query
	for foundCloserNode {
		for _, nodeToQuery := range snapshot {
			result, err := node.Network.FindNode(node.Self, nodeToQuery, targetID)

			if err != nil {
				continue
			}

			for _, contact := range result {
				node.RoutingTable.Update(contact)
			}

			// If the newly queried contact doesn't know a closer node to the target id, exit the loop
			if XorDistance(shortlist.Contacts[0].ID, targetID).Cmp(XorDistance(result[0].ID, targetID)) == -1 {
				foundCloserNode = false
				break
			}

			shortlist.Contacts = append(shortlist.Contacts, result...)
			queried[nodeToQuery.ID] = true
		}

		if !foundCloserNode || len(snapshot) == 0 {
			break
		}

		// Clear the snapshot
		snapshot = make([]Contact, 0, alphaConcurrency)

		// Retake the snapshot
		for _, nodeToQuery := range shortlist.Contacts {
			if len(snapshot) >= alphaConcurrency {
				break
			}

			if !queried[nodeToQuery.ID] {
				snapshot = append(snapshot, nodeToQuery)
			}
		}
	}

	sort.Sort(shortlist)
	shortlist.Print()

	if len(shortlist.Contacts) < k {
		return shortlist.Contacts
	}

	return shortlist.Contacts[:k]

}

func (node *Node) Join(bootstrapNode Contact) error {
	node.RoutingTable.Update(bootstrapNode)

	node.Lookup(node.Self.ID)

	return nil
}
