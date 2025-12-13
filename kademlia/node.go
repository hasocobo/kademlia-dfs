package kademliadfs

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"sort"
	"sync"
	"time"
)

type Node struct {
	Self         Contact
	RoutingTable *RoutingTable
	Network      Network
	Storage      map[NodeId][]byte

	mu sync.RWMutex
}

type LookupResult struct {
	NodeId   NodeId
	Contacts []Contact
	Value    []byte
}

// type ConcurrentQuery func(chan LookupResult)

func NewNode(id NodeId, ip net.IP, port int, network Network) *Node {
	contact := Contact{ID: id, IP: ip, Port: port}
	rt := NewRoutingTable(id, func(recipient Contact) bool {
		err := network.Ping(contact, recipient)
		return err == nil
	})
	storage := make(map[NodeId][]byte)

	return &Node{Self: contact, RoutingTable: rt, Storage: storage, Network: network}
}

type NodeId [idLength]byte

func NewNodeId(name string) NodeId {
	return sha256.Sum256([]byte(name))
}

func NewRandomId() NodeId {
	var randomID NodeId
	_, err := rand.Read(randomID[:]) // This fills randomId variable with random bytes
	if err != nil {
		panic(fmt.Sprintf("failed to generate random NodeId: %v", err))
	}
	return randomID
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
func (node *Node) HandleFindNode(requester Contact, key NodeId) []Contact {
	node.RoutingTable.Update(requester)

	return node.RoutingTable.FindClosest(key, k)
}

// Return the value if it exists, return closest nodes if not
func (node *Node) HandleFindValue(requester Contact, key NodeId) ([]byte, []Contact) {
	node.RoutingTable.Update(requester)

	node.mu.Lock()
	defer node.mu.Unlock()
	if value, ok := node.Storage[key]; ok {
		return value, nil
	}

	closestContacts := node.RoutingTable.FindClosest(key, k)
	return nil, closestContacts
}

func (node *Node) HandleStore(requester Contact, key NodeId, value []byte) {
	node.RoutingTable.Update(requester)

	node.mu.RLock()
	defer node.mu.RUnlock()

	node.Storage[key] = value
}

func (node *Node) Lookup(targetID NodeId) []Contact {
	resultsChan := make(chan LookupResult, maxConcurrentRequests)

	// Grabs a few nodes from its own buckets for initial population
	shortlist := NewContactSorter(targetID)
	shortlist.Add(node.RoutingTable.FindClosest(targetID, maxConcurrentRequests)...)

	// Number of current concurrent requests
	inFlightCounter := 0

	query := func(c chan LookupResult, nodeToQuery Contact) {
		result, err := node.Network.FindNode(node.Self, nodeToQuery, targetID)
		if err != nil {
			log.Printf("error finding node: %v", err)
			c <- LookupResult{NodeId: nodeToQuery.ID, Contacts: nil}
			return
		}
		c <- LookupResult{NodeId: nodeToQuery.ID, Contacts: result}
	}

	findNextUnqueriedContact := func(contactList []Contact, queryMap map[NodeId]bool) (*Contact, error) {
		for _, contact := range contactList {
			if queryMap[contact.ID] {
				continue
			}
			queryMap[contact.ID] = true
			return &contact, nil
		}
		return nil, fmt.Errorf("Contact Not Found")
	}

	if len(shortlist.Contacts) == 0 {
		fmt.Println("Found no contacts in buckets")
		return nil
	}

	sort.Sort(shortlist)

	queried := make(map[NodeId]bool, k)

	moreNodesToQuery := true
	// Then queries these nodes and keeps populating nodes to query
lookupLoop:
	for moreNodesToQuery {
		for inFlightCounter < maxConcurrentRequests {
			nodeToQuery, err := findNextUnqueriedContact(shortlist.Contacts, queried)
			if err != nil {
				break
			}
			inFlightCounter++
			go query(resultsChan, *nodeToQuery)
		}

		select {
		case <-time.After(time.Millisecond * 10000):
			moreNodesToQuery = false
			fmt.Println("query request timed out")
			break lookupLoop
		case result := <-resultsChan:
			inFlightCounter--
			for _, contact := range result.Contacts {
				node.RoutingTable.Update(contact)
			}
			// If the newly queried contact doesn't know a closer node to the target id, exit the loop
			if len(result.Contacts) == 0 ||
				XorDistance(shortlist.Contacts[0].ID, targetID) < XorDistance(result.Contacts[0].ID, targetID) {
				// If there are still concurrent requests at the moment, don't immediately break the loop
				// and keep going
				if inFlightCounter > 0 {
					continue
				}
				moreNodesToQuery = false
				break lookupLoop
			}

			shortlist.Add(result.Contacts...)

			nodeToQuery, err := findNextUnqueriedContact(shortlist.Contacts, queried)
			if err != nil {
				log.Print("there are no nodes left to lookup")
				break lookupLoop
			}

			inFlightCounter++
			go query(resultsChan, *nodeToQuery)
		}

	}

	if len(shortlist.Contacts) < k {
		return shortlist.Contacts
	}

	return shortlist.Contacts[:k]
}

func (node *Node) ValueLookup(key NodeId) (contacts []Contact, value []byte, err error) {
	resultsChan := make(chan LookupResult, maxConcurrentRequests)

	// Grabs a few nodes from its own buckets for initial population
	shortlist := NewContactSorter(key)
	shortlist.Add(node.RoutingTable.FindClosest(key, maxConcurrentRequests)...)

	// Number of current concurrent requests
	inFlightCounter := 0

	query := func(c chan LookupResult, nodeToQuery Contact) error {
		value, contacts, err := node.Network.FindValue(node.Self, nodeToQuery, key)
		if err != nil {
			log.Fatalf("error finding node: %v", err)
			return fmt.Errorf("error finding value: %v", err)
		}
		c <- LookupResult{NodeId: nodeToQuery.ID, Contacts: contacts, Value: value}
		return nil
	}

	findNextUnqueriedContact := func(contactList []Contact, queryMap map[NodeId]bool) (*Contact, error) {
		for _, contact := range contactList {
			if queryMap[contact.ID] {
				continue
			}
			queryMap[contact.ID] = true
			return &contact, nil
		}
		return nil, fmt.Errorf("not found any contact")
	}

	if len(shortlist.Contacts) == 0 {
		return nil, nil, fmt.Errorf("found no contacts in buckets")
	}

	sort.Sort(shortlist)

	queried := make(map[NodeId]bool, k)

	moreNodesToQuery := true
	// Then queries these nodes and keeps populating nodes to query
lookupLoop:
	for moreNodesToQuery {
		for inFlightCounter < maxConcurrentRequests {
			nodeToQuery, err := findNextUnqueriedContact(shortlist.Contacts, queried)
			if err != nil {
				break lookupLoop
			}
			inFlightCounter++
			go query(resultsChan, *nodeToQuery)
		}

		select {
		case <-time.After(time.Millisecond * 10000):
			moreNodesToQuery = false
			fmt.Println("query request timed out")
			break lookupLoop
		case result := <-resultsChan:
			inFlightCounter--
			if result.Value != nil {
				log.Println("found the value")
				return nil, result.Value, nil
			}

			for _, contact := range result.Contacts {
				node.RoutingTable.Update(contact)
			}
			// If the newly queried contact doesn't know a closer node to the target id, exit the loop
			if len(result.Contacts) == 0 ||
				XorDistance(shortlist.Contacts[0].ID, key) < XorDistance(result.Contacts[0].ID, key) {
				// If there are still concurrent requests at the moment, don't immediately break the loop
				// and keep going
				if inFlightCounter > 0 {
					continue
				}
				moreNodesToQuery = false
				break lookupLoop
			}

			shortlist.Add(result.Contacts...)

			nodeToQuery, err := findNextUnqueriedContact(shortlist.Contacts, queried)
			if err != nil {
				break lookupLoop
			}

			inFlightCounter++
			go query(resultsChan, *nodeToQuery)
		}

	}

	if len(shortlist.Contacts) < k {
		return shortlist.Contacts, nil, nil
	}

	return shortlist.Contacts[:k], nil, nil
}

func (node *Node) Join(bootstrapNode Contact) error {
	node.RoutingTable.Update(bootstrapNode)

	node.Lookup(node.Self.ID)

	return nil
}

func (node *Node) Get(key string) ([]byte, error) {
	hashedKey := sha256.Sum256([]byte(key))
	_, value, err := node.ValueLookup(hashedKey)
	if err != nil {
		return nil, fmt.Errorf("error finding value for key")
	}
	if value != nil {
		return value, nil
	}
	return nil, nil
}

func (node *Node) Put(key string, value []byte) error {
	hashedKey := sha256.Sum256([]byte(key))
	closestContacts := node.Lookup(hashedKey)

	if len(closestContacts) == 0 {
		return fmt.Errorf("error finding closest nodes for put: length is equal to 0")
	}

	for _, contact := range closestContacts {
		node.Network.Store(node.Self, contact, hashedKey, value)
	}
	return nil
}
