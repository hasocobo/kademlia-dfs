package kademliadfs

import (
	"container/list"
	"sort"
	"sync"
)

type RoutingTable struct {
	Buckets [idLength * 8]Bucket
	SelfID  NodeId

	Ping pingFunc
}

type Bucket struct {
	// Using list.List(linked-list) for LRU management and efficient insertion/deletion
	// We use []Contact slices(array-list) when sorting for lookups
	Contacts *list.List
	mu       sync.RWMutex
}

func NewRoutingTable(selfID NodeId, ping pingFunc) *RoutingTable {
	rt := &RoutingTable{SelfID: selfID, Ping: ping}

	for i := range len(rt.Buckets) {
		rt.Buckets[i].Contacts = list.New()
	}
	return rt
}

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

type pingFunc func(Contact) bool

/*
We dump all of the contacts from each bucket into the contact sorter and
then call its sort function to sort their distance to the targetID
*/
func (rt *RoutingTable) FindClosest(targetID NodeId, count int) []Contact {
	// TODO: remove O(n) iteration and replace it with spiraling(i + 1, i - 1, i + 2, i - 2) starting from buckets[determinedIndexByXorOfTargetAndSelfId]
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

func (rt *RoutingTable) GetBucketIndex(selfID, targetID NodeId) int {
	dist := xorDistance(selfID, targetID)

	return dist.BitLen() - 1
}
