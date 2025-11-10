package main

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"math/big"
	"net"
	"sort"
	"sync"
)

/*
We use idLength = 32 to match the 32 bytes produced by SHA-256 hashes.
SHA-256 is preferred over SHA-1 for new Kademlia implementations because SHA-1 is now considered cryptographically broken—
practical collision attacks have been demonstrated against SHA-1, meaning two different inputs can be crafted to produce the same SHA-1 hash.
This makes the identifier space vulnerable to attacks and undermines the security of the DHT.
SHA-256 is much stronger, with no known practical collision or preimage attacks, it is a safer choice for node identifiers.
*/
const (
	idLength = 32
	k        = 32
)

type NodeId [idLength]byte

func NewNodeId(name string) NodeId {
	return sha256.Sum256([]byte(name))
}

func (nodeId NodeId) String() string {
	return hex.EncodeToString(nodeId[:])
}

func xorDistance(a, b NodeId) *big.Int {
	ai := new(big.Int).SetBytes(a[:])
	bi := new(big.Int).SetBytes(b[:])
	return new(big.Int).Xor(ai, bi)
}

type Contact struct {
	ID   NodeId
	IP   net.IP
	Port int
}

type Bucket struct {
	// Using list.List(linked-list) for LRU management and efficient insertion/deletion
	// We use []Contact slices(array-list) when sorting for lookups
	Contacts *list.List
	mu       sync.RWMutex
}

type RoutingTable struct {
	Buckets [idLength * 8]Bucket
	SelfID  NodeId
	Ping    pingFunc
}

type pingFunc func(Contact) bool

func (rt *RoutingTable) Update(c Contact) {
	idx := rt.GetBucketIndex(rt.SelfID, c.ID)
	bucket := &rt.Buckets[idx]

	bucket.mu.Lock()
	defer bucket.mu.Unlock()
	// move to front if the contact exists, add to contacts if not
	for e := bucket.Contacts.Front(); e != nil; e = e.Next() {
		if e.Value.(Contact).ID == c.ID {
			bucket.Contacts.MoveToFront(e)
			return
		}
	}
	// if it's a new contact and the bucket is full
	if bucket.Contacts.Len() == k {
		if !rt.Ping(bucket.Contacts.Back().Value.(Contact)) {
			lruElem := bucket.Contacts.Back()
			bucket.Contacts.Remove(lruElem)
			bucket.Contacts.PushFront(c)
		} else {
			bucket.Contacts.MoveToFront(bucket.Contacts.Back())
		}
		return
	}

	bucket.Contacts.PushFront(c)
}

func (rt *RoutingTable) GetBucketIndex(selfID, targetID NodeId) int {
	dist := xorDistance(selfID, targetID)

	return dist.BitLen() - 1
}

/*
We dump all of the contacts from each bucket into the contact sorter and
then call its sort function to sort their distance to the targetID
*/
func (rt *RoutingTable) FindClosest(targetID NodeId, count int) []Contact {
	contactSorter := &ContactSorter{TargetID: targetID}
	for i := range rt.Buckets {
		bucket := &rt.Buckets[i]

		for e := bucket.Contacts.Front(); e != nil; e = e.Next() {
			contactSorter.Contacts = append(contactSorter.Contacts, e.Value.(Contact))
		}

	}

	sort.Sort(contactSorter)

	if len(contactSorter.Contacts) < count {
		return contactSorter.Contacts[:]
	}
	return contactSorter.Contacts[:count]
}

func NewRoutingTable(selfID NodeId, ping pingFunc) *RoutingTable {
	rt := &RoutingTable{SelfID: selfID, Ping: ping}

	for i := range len(rt.Buckets) {
		rt.Buckets[i].Contacts = list.New()
	}
	return rt
}

type ContactSorter struct {
	TargetID NodeId
	Contacts []Contact
}

// sort.Interface implementation
func (cs ContactSorter) Len() int { return len(cs.Contacts) }
func (cs ContactSorter) Swap(i, j int) {
	cs.Contacts[i], cs.Contacts[j] = cs.Contacts[j], cs.Contacts[i]
}

func (cs ContactSorter) Less(i, j int) bool {
	distI := xorDistance(cs.Contacts[i].ID, cs.TargetID)
	distJ := xorDistance(cs.Contacts[j].ID, cs.TargetID)
	return distI.Cmp(distJ) == -1
}

func main() {}
