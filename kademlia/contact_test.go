package kademliadfs

import (
	"reflect"
	"testing"
)

func TestContactSorter_AddDuplicateEntries_DeduplicatesThem(t *testing.T) {
	t.Parallel()
	targetId := NewNodeId("test")
	contactSorter := NewContactSorter(targetId)

	for i := range 9 {
		if i >= 0 && i < 3 {
			contactSorter.Add(Contact{ID: NewNodeId("Test 1")})
		} else if i >= 3 && i < 6 {
			contactSorter.Add(Contact{ID: NewNodeId("Test 2")})
		} else if i >= 6 && i < 9 {
			contactSorter.Add(Contact{ID: NewNodeId("Test 3")})
		}
	}

	if contactSorter.Len() != 3 {
		t.Fatalf("Unexpected contact sorter length. Want: %v, Got: %v", 3, contactSorter.Len())
	}
}

func TestContactSorter_AddMultipleEntries_SortsEntries(t *testing.T) {
	t.Parallel()

	targetId := NewNodeId("test")
	contactSorter := NewContactSorter(targetId)

	closestContact := CreateContactForBucket(targetId, 1, 1)
	middleContact := CreateContactForBucket(targetId, 128, 1)
	furthestContact := CreateContactForBucket(targetId, 255, 1)

	contactSorter.Add(furthestContact)
	contactSorter.Add(closestContact)
	contactSorter.Add(middleContact)

	if !reflect.DeepEqual(contactSorter.Get(0), closestContact) ||
		!reflect.DeepEqual(contactSorter.Get(1), middleContact) ||
		!reflect.DeepEqual(contactSorter.Get(2), furthestContact) {

		t.Fatalf("Contacts not sorted: got = [%+v, %+v, %+v], want = [%+v, %+v, %+v]",
			contactSorter.Get(0), contactSorter.Get(1), contactSorter.Get(2),
			closestContact, middleContact, furthestContact)
	}

}
