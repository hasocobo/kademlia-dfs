package kademliadfs

import (
	"fmt"
	"net"
	"sort"
	"sync"
)

type Contact struct {
	ID   NodeId
	IP   net.IP
	Port int
}

type ContactSorter struct {
	TargetID NodeId
	Contacts []Contact

	// We need this to know if an element is already in the contacts,
	// required to prevent duplications
	IsInContacts map[NodeId]bool

	mu sync.Mutex
}

func NewContactSorter(targetID NodeId) *ContactSorter {
	return &ContactSorter{TargetID: targetID, IsInContacts: make(map[NodeId]bool)}
}

func (cs *ContactSorter) Add(c ...Contact) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	for _, contact := range c {
		if _, exists := cs.IsInContacts[contact.ID]; exists {
			continue
		}

		cs.Contacts = append(cs.Contacts, contact)
		cs.IsInContacts[contact.ID] = true
	}
	// TODO: Remove sort.Sort and replace it with binary search adding algorithm
	sort.Sort(cs)
}

func (cs *ContactSorter) Get(i int) Contact {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	return cs.Contacts[i]
}

func (cs *ContactSorter) Print() {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	for _, contact := range cs.Contacts {
		fmt.Printf("ID: %v, Distance: %v", contact.ID, XorDistance(cs.TargetID, contact.ID))
		fmt.Println()
	}
}

// sort.Interface methods

func (cs *ContactSorter) Len() int { return len(cs.Contacts) }

func (cs *ContactSorter) Swap(i, j int) {
	cs.Contacts[i], cs.Contacts[j] = cs.Contacts[j], cs.Contacts[i]
}

func (cs *ContactSorter) Less(i, j int) bool {
	a := cs.Contacts[i].ID
	b := cs.Contacts[j].ID
	target := cs.TargetID

	for idx := range idLength {
		distI := a[idx] ^ target[idx]
		distJ := b[idx] ^ target[idx]

		if distI < distJ {
			return true
		} else if distI > distJ {
			return false
		}
	}
	return false
}
