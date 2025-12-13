package kademliadfs

import (
	"container/list"
	"net"
	"reflect"
	"testing"
)

func BenchmarkFindClosest(b *testing.B) {
	selfID := NewNodeId("test-id")
	targetId := NewNodeId("target-id")
	rt := NewRoutingTable(selfID, func(c Contact) bool { return false }) // dummy ping returns false
	for i := range idLength * 8 {
		dummyContacts := list.New()
		for range k {
			dummyContacts.PushFront(Contact{ID: NewRandomId()})
		}
		rt.Buckets[i].Contacts = dummyContacts
	}

	for b.Loop() {
		rt.FindClosest(targetId, k)
	}
}

func TestRoutingTableUpdate_NewContact_BucketNotFull(t *testing.T) {
	t.Parallel()

	selfID := NewNodeId("test-id")
	testBucketIndex := 10
	contact := CreateContactForBucket(selfID, testBucketIndex, 0)

	rt := NewRoutingTable(selfID, func(c Contact) bool { return false }) // dummy ping returns false

	rt.Update(contact)

	if rt.Buckets[testBucketIndex].Contacts.Len() == 0 {
		t.Fatalf("bucket %d is empty after Update", testBucketIndex)
	}

	if !reflect.DeepEqual(rt.Buckets[testBucketIndex].Contacts.Front().Value, contact) {
		t.Fatalf("unexpected contact in bucket %d: got %+v, want %+v", testBucketIndex, rt.Buckets[testBucketIndex].Contacts.Front().Value, contact)
	}
}

func TestRoutingTableUpdate_ExistingContact_ShouldMoveToFront(t *testing.T) {
	t.Parallel()

	selfID := NewNodeId("test-id")
	testBucketIndex := 10

	rt := NewRoutingTable(selfID, func(c Contact) bool { return false }) // dummy ping returns false

	existingContact := CreateContactForBucket(selfID, testBucketIndex, 0)
	newContact := CreateContactForBucket(selfID, testBucketIndex, 1)

	rt.Buckets[testBucketIndex].Contacts.PushFront(existingContact)
	rt.Buckets[testBucketIndex].Contacts.PushFront(newContact)

	if rt.Buckets[testBucketIndex].Contacts.Len() != 2 {
		t.Fatalf("unexpected bucket size, want 2, got: %d", rt.Buckets[testBucketIndex].Contacts.Len())
	}

	if reflect.DeepEqual(rt.Buckets[testBucketIndex].Contacts.Front().Value, existingContact) {
		t.Fatalf("front value is already the existing contact")
	}

	rt.Update(existingContact)

	if !reflect.DeepEqual(rt.Buckets[testBucketIndex].Contacts.Front().Value, existingContact) {
		t.Fatalf("unexpected contact in bucket %d: got %+v, want %+v",
			testBucketIndex, rt.Buckets[testBucketIndex].Contacts.Front().Value, existingContact)
	}
}

func TestRoutingTableUpdate_NewContact_BucketFull_PingFails_ShouldReplaceExisting(t *testing.T) {
	t.Parallel()

	selfID := NewNodeId("test-id")
	testBucketIndex := 10

	rt := NewRoutingTable(selfID, func(c Contact) bool { return false }) // dummy ping returns false

	existingContact := CreateContactForBucket(selfID, testBucketIndex, 0)
	contactToBeEjected := CreateContactForBucket(selfID, testBucketIndex, 1)
	newContact := CreateContactForBucket(selfID, testBucketIndex, 2)

	testBucket := &rt.Buckets[testBucketIndex]
	testBucket.Contacts.PushBack(contactToBeEjected)
	for i := 0; i < k-1; i++ {
		rt.Buckets[testBucketIndex].Contacts.PushFront(existingContact)
	}

	rt.Update(newContact)

	if rt.Buckets[testBucketIndex].Contacts.Len() != k {
		t.Fatalf("unexpected bucket size, want: %d, got: %d", k, rt.Buckets[testBucketIndex].Contacts.Len())
	}

	if !reflect.DeepEqual(rt.Buckets[testBucketIndex].Contacts.Front().Value, newContact) {
		t.Fatalf("unexpected front contact value. want: %+v, got: %+v", newContact, rt.Buckets[testBucketIndex].Contacts.Front().Value)
	}

	if backValue := rt.Buckets[testBucketIndex].Contacts.Back().Value; reflect.DeepEqual(backValue, contactToBeEjected) {
		t.Fatalf(
			"unexpected contact in bucket %d: failed to eject the lru contact. got %+v, want %+v",
			testBucketIndex, backValue, existingContact)
	}
}

func TestRoutingTableUpdate_NewContact_BucketFull_PingSucceeds_ShouldKeepExisting(t *testing.T) {
	t.Parallel()

	selfID := NewNodeId("test-id")
	testBucketIndex := 10

	rt := NewRoutingTable(selfID, func(c Contact) bool { return true }) // dummy ping returns true

	existingContact := CreateContactForBucket(selfID, testBucketIndex, 0)
	contactToBeKept := CreateContactForBucket(selfID, testBucketIndex, 1)
	newContact := CreateContactForBucket(selfID, testBucketIndex, 2)

	for i := range k {
		if i == k-1 {
			rt.Buckets[testBucketIndex].Contacts.PushBack(contactToBeKept)
			continue
		}
		rt.Buckets[testBucketIndex].Contacts.PushFront(existingContact)
	}

	rt.Update(newContact)

	if rt.Buckets[testBucketIndex].Contacts.Len() != k {
		t.Fatalf("unexpected bucket size, want: %d, got: %d", k, rt.Buckets[testBucketIndex].Contacts.Len())
	}

	if !reflect.DeepEqual(rt.Buckets[testBucketIndex].Contacts.Front().Value, contactToBeKept) {
		t.Fatalf("unexpected front contact value. want: %+v, got: %+v", contactToBeKept, rt.Buckets[testBucketIndex].Contacts.Front().Value)
	}

	if backValue := rt.Buckets[testBucketIndex].Contacts.Back().Value; reflect.DeepEqual(backValue, contactToBeKept) {
		t.Fatalf(
			"unexpected contact in bucket %d: failed to move the lru contact. got %+v, want %+v",
			testBucketIndex, backValue, existingContact)
	}

	for e := rt.Buckets[testBucketIndex].Contacts.Front(); e != nil; e = e.Next() {
		if reflect.DeepEqual(e.Value, newContact) {
			t.Fatalf("newContact was not rejected; it was found in the bucket")
		}
	}
}

func TestRoutingTableFindClosest_MoreThanKContacts_PingReturnsTrue_ShouldReturnFirstKContacts(t *testing.T) {
	t.Parallel()

	numberOfContacts := max(k*2, 2) // Needs at least 2 contacts

	selfID := NewNodeId("test-id")
	targetID := NodeId{}                                                // all zeros for easy testability
	rt := NewRoutingTable(selfID, func(c Contact) bool { return true }) // dummy ping returns true

	// Create a Node ID with 1 at the index while leaving the rest 0
	// Need to use this to create one node for each bucket
	// TODO: Replace this with already existing CreateBucketForIndex function in contact_test.go
	createNodeIDWithBitSet := func(bitIndex int) NodeId {
		tempID := NodeId{}
		byteIndex := idLength - 1 - (bitIndex / 8)
		bitPosition := bitIndex % 8

		tempID[byteIndex] = 1 << bitPosition

		return tempID
	}

	expectedContacts := make([]Contact, k)

	for i := range k {
		expectedContacts[i] = Contact{ID: createNodeIDWithBitSet(i), Port: 10000 + i, IP: net.IPv4(127, 0, 0, 1)}
	}

	for i := range numberOfContacts {
		rt.Update(Contact{ID: createNodeIDWithBitSet(i), Port: 10000 + i, IP: net.IPv4(127, 0, 0, 1)})
	}

	closestContacts := rt.FindClosest(targetID, k)

	if len(closestContacts) != k {
		t.Fatalf("unexpected length. got: %d, want: %d", len(closestContacts), k)
	}

	if len(closestContacts) != len(expectedContacts) {
		t.Fatalf("length of closestContacts does not match length of expectedContacts. got: %d, want: %d", len(closestContacts), len(expectedContacts))
	}

	// if the first element's distance isn't less than that of the second element, fail
	if XorDistance(closestContacts[0].ID, targetID).Cmp(XorDistance(closestContacts[1].ID, targetID)) != -1 {
		t.Fatalf("contacts are not sorted by ascending distance to targetID: dist[0]=%v, dist[1]=%v, contact[0]=%+v, contact[1]=%+v",
			XorDistance(closestContacts[0].ID, targetID), XorDistance(closestContacts[1].ID, targetID), closestContacts[0], closestContacts[1])
	}

	if !reflect.DeepEqual(closestContacts, expectedContacts) {
		t.Fatalf("contacts do not match expected.\nGot: %+v\nWant: %+v", closestContacts, expectedContacts)
	}
}

func TestRoutingTableFindClosest_LessThanKContacts_ShouldReturnAll(t *testing.T) {
	t.Parallel()

	numberOfContacts := max(k/2, 2) // Needs at least 2 contacts

	selfID := NewNodeId("test-id")
	targetID := NodeId{}                                                 // all zeros for easy testability
	rt := NewRoutingTable(selfID, func(c Contact) bool { return false }) // dummy ping returns false

	// Create a Node ID with 1 at the index while leaving the rest 0
	// Need to use this to create one node for each bucket
	createNodeIDWithBitSet := func(bitIndex int) NodeId {
		tempID := NodeId{}
		byteIndex := idLength - 1 - (bitIndex / 8)
		bitPosition := bitIndex % 8

		tempID[byteIndex] = 1 << bitPosition

		return tempID
	}

	expectedContacts := make([]Contact, numberOfContacts)

	for i := range numberOfContacts {
		expectedContacts[i] = Contact{ID: createNodeIDWithBitSet(i), Port: 10000 + i, IP: net.IPv4(127, 0, 0, 1)}
	}

	for i := range numberOfContacts {
		rt.Update(Contact{ID: createNodeIDWithBitSet(i), Port: 10000 + i, IP: net.IPv4(127, 0, 0, 1)})
	}

	closestContacts := rt.FindClosest(targetID, numberOfContacts)

	if len(closestContacts) != numberOfContacts {
		t.Fatalf("unexpected length. got: %d, want: %d", len(closestContacts), numberOfContacts)
	}

	// if the first element's distance isn't less than that of the second element, fail
	if XorDistance(closestContacts[0].ID, targetID).Cmp(XorDistance(closestContacts[1].ID, targetID)) != -1 {
		t.Fatalf("contacts are not sorted by ascending distance to targetID: dist[0]=%v, dist[1]=%v, contact[0]=%+v, contact[1]=%+v",
			XorDistance(closestContacts[0].ID, targetID), XorDistance(closestContacts[1].ID, targetID), closestContacts[0], closestContacts[1])
	}

	if !reflect.DeepEqual(closestContacts, expectedContacts) {
		t.Fatalf("contacts do not match expected.\nGot: %+v\nWant: %+v", closestContacts, expectedContacts)
	}
}
