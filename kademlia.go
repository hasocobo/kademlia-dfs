package main

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"net"
	"sort"
	"strconv"
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
	fmt.Println(contactSorter.Contacts)

	if len(contactSorter.Contacts) < count {
		return contactSorter.Contacts[:]
	}
	return contactSorter.Contacts[:count]
}

func (rt *RoutingTable) Print() {
	for i, bucket := range rt.Buckets {
		bucket.mu.RLock() // Read lock

		// Skip empty buckets for cleaner output
		if bucket.Contacts.Len() == 0 {
			bucket.mu.RUnlock()
			continue
		}

		fmt.Printf("Bucket %d (%d contacts):\n", i, bucket.Contacts.Len())

		// Iterate through the actual list elements
		for e := bucket.Contacts.Front(); e != nil; e = e.Next() {
			contact := e.Value.(Contact)
			fmt.Printf("  ID: %s, IP: %s, Port: %d\n",
				contact.ID.String()[:8]+"...", // First 8 chars of hash
				contact.IP,
				contact.Port)
		}

		bucket.mu.RUnlock()
	}
}

func NewRoutingTable(selfID NodeId) *RoutingTable {
	rt := &RoutingTable{SelfID: selfID}

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

// To determine what bucket the target node belongs to, we need to get the index of the distance (value we get after applying xorDistance),
//
// i.e, 010001 means it belongs to the 5th bucket(read from right to left, least significant bit is at the most right)
//
// So if we calculate it with log_2, we get the index.
//
// But since sha1 is 160-bit length, we can't use math.log2 because it accepts numbers with 64 bit,
//
// So we have to use big.Int.BitLen which does the same thing on larger numbers

func (cs ContactSorter) Print() {
	for _, v := range cs.Contacts {
		fmt.Println("Contact: ", v, "Distance: ", xorDistance(v.ID, cs.TargetID))
	}
}

func main() {

	selfID := NewNodeId("test")
	rt := NewRoutingTable(selfID)

	rt.Print()

	for i := range 10 {
		rt.Update(Contact{ID: NewNodeId("test " + strconv.Itoa(i)), IP: net.IPv4zero, Port: 10000 + i})
	}

	fmt.Println("After 10 Contacts")

	rt.Print()

	fmt.Println("K closest contacts after 10 Contacts")
	closestContacts := rt.FindClosest(NewNodeId("test3"), k)

	for _, v := range closestContacts {
		fmt.Printf("Closest: %s\n Distance: %s\n", v.ID.String(), xorDistance(NewNodeId("test3"), v.ID))

	}

	for i := range 100 {
		rt.Update(Contact{ID: NewNodeId("test " + strconv.Itoa(i)), IP: net.IPv4zero, Port: 10000 + i})
	}

	fmt.Println("After 110 Contacts")
	rt.Print()

	fmt.Println("K closest contacts after 110 Contacts")
	closestContacts = rt.FindClosest(NewNodeId("test3"), k)

	for _, v := range closestContacts {
		fmt.Printf("Closest: %s\n Distance: %s\n", v.ID.String(), xorDistance(NewNodeId("test3"), v.ID))

	}

	// for i := range 10000 {
	// 	rt.Update(Contact{ID: NewNodeId("test " + strconv.Itoa(i)), IP: net.IPv4zero, Port: 10000 + i})
	// }

	// fmt.Println("After 10110 Contacts")
	// rt.Print()

}
