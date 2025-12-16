package kademliadfs

import (
	"fmt"
	"net"
	"sync"
)

type Contact struct {
	ID   NodeId
	IP   net.IP
	Port int
}

type ContactSorter struct {
	TargetID NodeId
	Contacts [k]Contact
	Length   int

	mu sync.Mutex
}

func NewContactSorter(targetID NodeId) *ContactSorter {
	return &ContactSorter{TargetID: targetID}
}

func (cs *ContactSorter) Add(c ...Contact) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	for _, contact := range c {
		for i := range cs.Length {
			if contact.ID == cs.Contacts[i].ID {
				continue
			}
		}

		if cs.Length == 0 {
			cs.Contacts[0] = contact
			cs.Length++
			continue
		}

		dist := XorDistance(contact.ID, cs.TargetID)
		if dist >= XorDistance(cs.Contacts[cs.Length-1].ID, cs.TargetID) {
			if cs.Length == k {
				continue
			}
			cs.Contacts[cs.Length] = contact
			cs.Length++
			continue
		}

		// 24
		// 19 22 23 44
		// i = 3
		for i := 0; i < cs.Length; i++ {
			if dist < XorDistance(cs.Contacts[i].ID, cs.TargetID) {
				if cs.Length == k {
					// shift right the array
					copy(cs.Contacts[i+1:], cs.Contacts[i:cs.Length-1])
				} else {
					copy(cs.Contacts[i+1:], cs.Contacts[i:cs.Length])
					cs.Length++
				}
				cs.Contacts[i] = contact
				break
			}
		}

	}
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

func (cs *ContactSorter) Len() int { return cs.Length }

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
