package kademliadfs

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"sync"
)

type Node struct {
	Self         Contact
	RoutingTable *RoutingTable
	Network      Network
	Storage      map[NodeId][]byte

	state NodeState
	mu    sync.RWMutex
}

type LookupResult struct {
	NodeId   NodeId
	Contacts []Contact
	Value    []byte
}

func NewNode(ctx context.Context, id NodeId, ip net.IP, port int, network Network) *Node {
	contact := Contact{ID: id, IP: ip, Port: port}
	rt := NewRoutingTable(id, func(ctx context.Context, recipient Contact) bool {
		err := network.Ping(ctx, contact, recipient)
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
		// TODO: add error returns
		fmt.Sprintf("failed to generate random NodeId: %v", err)
	}
	return randomID
}

func (nodeId NodeId) String() string {
	return hex.EncodeToString(nodeId[:])
}

func (node *Node) HandlePing(ctx context.Context, requester Contact) bool {
	node.RoutingTable.Update(ctx, requester)

	return true
}

func (node *Node) HandleFindNode(ctx context.Context, requester Contact, key NodeId) []Contact {
	if ctx.Err() != nil {
		return nil
	}

	node.RoutingTable.Update(ctx, requester)

	return node.RoutingTable.FindClosest(key, k)
}

// HandleFindValue returns the value if it exists, return closest nodes if not
func (node *Node) HandleFindValue(ctx context.Context, requester Contact, key NodeId) ([]byte, []Contact) {
	if ctx.Err() != nil {
		return nil, nil
	}

	node.RoutingTable.Update(ctx, requester)

	node.mu.Lock()
	defer node.mu.Unlock()
	if value, ok := node.Storage[key]; ok {
		return value, nil
	}

	closestContacts := node.RoutingTable.FindClosest(key, k)
	return nil, closestContacts
}

func (node *Node) HandleStore(ctx context.Context, requester Contact, key NodeId, value []byte) {
	if ctx.Err() != nil {
		return
	}

	node.RoutingTable.Update(ctx, requester)

	node.mu.Lock()
	defer node.mu.Unlock()

	node.Storage[key] = value
}

func (node *Node) Lookup(ctx context.Context, targetID NodeId) []Contact {
	resultsChan := make(chan LookupResult, maxConcurrentRequests)

	// Grabs a few nodes from its own buckets for initial population
	shortlist := NewContactSorter(targetID)
	shortlist.Add(node.RoutingTable.FindClosest(targetID, maxConcurrentRequests)...)

	// Number of current concurrent requests
	inFlightCounter := 0

	query := func(c chan LookupResult, nodeToQuery Contact) {
		result, err := node.Network.FindNode(ctx, node.Self, nodeToQuery, targetID)
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

	if shortlist.Length == 0 {
		fmt.Println("Found no contacts in buckets")
		return nil
	}

	queried := make(map[NodeId]bool, k)

	moreNodesToQuery := true
	// Then queries these nodes and keeps populating nodes to query
lookupLoop:
	for moreNodesToQuery {
		for inFlightCounter < maxConcurrentRequests {
			nodeToQuery, err := findNextUnqueriedContact(shortlist.Contacts[:shortlist.Length], queried)
			if err != nil {
				break
			}
			inFlightCounter++
			go query(resultsChan, *nodeToQuery)
		}

		select {
		case <-ctx.Done():
			moreNodesToQuery = false
			fmt.Println("query request timed out")
			break lookupLoop
		case result := <-resultsChan:
			inFlightCounter--
			// If the newly queried contact doesn't know a closer node to the target id, exit the loop
			if len(result.Contacts) == 0 {
				// If there are still concurrent requests at the moment, don't immediately break the loop
				// and keep going
				if inFlightCounter > 0 {
					continue
				}
				moreNodesToQuery = false
				break lookupLoop
			}
			shortlist.Add(result.Contacts...)

			for _, contact := range result.Contacts {
				node.RoutingTable.Update(ctx, contact)
			}

			nodeToQuery, err := findNextUnqueriedContact(shortlist.Contacts[:shortlist.Length], queried)
			if err != nil {
				log.Print("there are no nodes left to lookup")
				break lookupLoop
			}

			inFlightCounter++
			go query(resultsChan, *nodeToQuery)
		}

	}

	return shortlist.Contacts[:shortlist.Length]
}

func (node *Node) ValueLookup(ctx context.Context, key NodeId) (contacts []Contact, value []byte, err error) {
	resultsChan := make(chan LookupResult, maxConcurrentRequests)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// Grabs a few nodes from its own buckets for initial population
	shortlist := NewContactSorter(key)
	shortlist.Add(node.RoutingTable.FindClosest(key, maxConcurrentRequests)...)

	// Number of current concurrent requests
	inFlightCounter := 0

	query := func(c chan LookupResult, nodeToQuery Contact) error {
		value, contacts, err := node.Network.FindValue(ctx, node.Self, nodeToQuery, key)
		if err != nil {
			log.Printf("error finding node: %v", err)
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

	if shortlist.Length == 0 {
		return nil, nil, fmt.Errorf("found no contacts in buckets")
	}

	queried := make(map[NodeId]bool, k)

	moreNodesToQuery := true
	// Then queries these nodes and keeps populating nodes to query
lookupLoop:
	for moreNodesToQuery {
		for inFlightCounter < maxConcurrentRequests {
			nodeToQuery, err := findNextUnqueriedContact(shortlist.Contacts[:shortlist.Length], queried)
			if err != nil {
				break lookupLoop
			}
			inFlightCounter++
			go query(resultsChan, *nodeToQuery)
		}

		select {
		case <-ctx.Done():
			moreNodesToQuery = false
			fmt.Println("query request timed out")
			break lookupLoop
		case result := <-resultsChan:
			inFlightCounter--
			if len(result.Value) != 0 {
				log.Printf("found the value: %v, length: %v\n", string(result.Value), len(result.Value))
				cancel() // cancel all other goroutines if the query finds the value
				return nil, result.Value, nil
			}

			// If the newly queried contact doesn't know a closer node to the target id, exit the loop
			if len(result.Contacts) == 0 {
				// If there are still concurrent requests at the moment, don't immediately break the loop
				// and keep going
				if inFlightCounter > 0 {
					continue
				}
				moreNodesToQuery = false
				break lookupLoop
			}
			shortlist.Add(result.Contacts...)

			for _, contact := range result.Contacts {
				node.RoutingTable.Update(ctx, contact)
			}

			nodeToQuery, err := findNextUnqueriedContact(shortlist.Contacts[:shortlist.Length], queried)
			if err != nil {
				break lookupLoop
			}

			inFlightCounter++
			go query(resultsChan, *nodeToQuery)
		}

	}

	return shortlist.Contacts[:shortlist.Length], nil, nil
}

func (node *Node) Join(ctx context.Context, bootstrapNode Contact) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	node.RoutingTable.Update(ctx, bootstrapNode)

	node.Lookup(ctx, node.Self.ID)

	return nil
}

func (node *Node) Get(ctx context.Context, key string) ([]byte, error) {
	hashedKey := sha256.Sum256([]byte(key))
	_, value, err := node.ValueLookup(ctx, hashedKey)
	if err != nil {
		return nil, fmt.Errorf("error finding value for key")
	}
	if value != nil {
		return value, nil
	}
	return nil, nil
}

func (node *Node) Put(ctx context.Context, key string, value []byte) error {
	hashedKey := sha256.Sum256([]byte(key))
	closestContacts := node.Lookup(ctx, hashedKey)

	if len(closestContacts) == 0 {
		return fmt.Errorf("error finding closest nodes for put: length is equal to 0")
	}

	for _, contact := range closestContacts {
		node.Network.Store(ctx, node.Self, contact, hashedKey, value)
	}
	return nil
}
