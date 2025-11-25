# Kademlia DHT Implementation - Detailed Repository Summary

## Overview

This repository contains a **Kademlia Distributed Hash Table (DHT)** implementation in Go. Kademlia is a peer-to-peer distributed hash table protocol that provides efficient node lookup, data storage, and retrieval in a decentralized network. This implementation focuses on the core DHT functionality with a simulation network for testing.

## Project Structure

```
kademlia-dfs/
├── main.go                    # Demo/example program
├── go.mod                     # Go module definition
├── kademlia-dfs               # Compiled binary (generated)
└── kademlia/                  # Core Kademlia package
    ├── kademlia.go           # Core constants and XOR distance function
    ├── node.go               # Node implementation and RPC handlers
    ├── routingtable.go       # Routing table with k-buckets
    ├── network.go            # Network interface and simulation network
    ├── contact.go            # Contact structure and sorting utilities
    └── kademlia_test.go      # Comprehensive unit tests
```

## Core Components

### 1. **Kademlia Protocol Constants** (`kademlia.go`)

- **Node ID Length**: 32 bytes (256 bits) - Uses SHA-256 for node identification
  - **Rationale**: SHA-256 is cryptographically secure (unlike SHA-1 which has known collision attacks)
  - Provides a massive identifier space: 2^256 possible node IDs
  
- **K Parameter**: 32
  - Maximum number of contacts stored per k-bucket
  - Balances redundancy and storage efficiency
  
- **Alpha Concurrency**: 3
  - Number of parallel queries during node lookup
  - Standard Kademlia parameter for balancing speed and network load

- **XOR Distance Metric**: 
  - Uses XOR operation on node IDs to calculate distance
  - Returns `*big.Int` for precise distance calculations
  - Forms the basis of the Kademlia routing algorithm

### 2. **Node Structure** (`node.go`)

A `Node` represents a peer in the Kademlia network with:

- **Self Contact**: The node's own contact information (ID, IP, Port)
- **Routing Table**: Manages k-buckets for efficient routing
- **Network Interface**: Abstraction for network communication
- **Storage**: In-memory key-value store (map[NodeId][]byte)
- **Mutex**: Thread-safe operations

#### Key Node Operations:

**RPC Handlers:**
- `HandlePing(Contact) bool`: Responds to ping requests, updates routing table
- `HandleFindNode(Contact, NodeId) []Contact`: Returns k closest nodes to target ID
- `HandleFindValue(Contact, NodeId) ([]byte, []Contact)`: Returns value if found, otherwise closest nodes
- `HandleStore(Contact, NodeId, []byte)`: Stores key-value pairs locally

**Core Algorithms:**
- `Lookup(NodeId) []Contact`: 
  - Implements iterative Kademlia lookup algorithm
  - Starts with α closest nodes from own routing table
  - Queries nodes in parallel, continuously finding closer nodes
  - Returns k closest nodes to target ID
  - O(log n) complexity

- `Join(Contact) error`: 
  - Bootstraps node into network
  - Updates routing table with bootstrap node
  - Performs self-lookup to populate routing table

### 3. **Routing Table** (`routingtable.go`)

The routing table is the heart of Kademlia's efficiency:

- **Structure**: 
  - 256 buckets (one for each bit position: idLength * 8)
  - Each bucket stores up to k=32 contacts
  - Uses Go's `container/list` for LRU (Least Recently Used) management

- **Bucket Index Calculation**:
  - Uses XOR distance between self ID and contact ID
  - Bucket index = bit length of XOR distance - 1
  - Ensures contacts are stored in appropriate buckets based on distance

- **Update Algorithm**:
  - If contact exists: Move to front (LRU update)
  - If bucket not full: Add to front
  - If bucket full: Ping least recently used contact
    - If ping fails: Replace with new contact
    - If ping succeeds: Move LRU to front, reject new contact

- **FindClosest Algorithm**:
  - Currently: O(n) iteration through all buckets
  - TODO: Optimize with spiral lookup starting from target bucket
  - Sorts contacts by XOR distance to target
  - Returns k closest contacts

### 4. **Network Layer** (`network.go`)

**Network Interface** defines 4 core RPC operations:
- `Ping(requester, recipient Contact) error`
- `FindNode(requester, recipient Contact, targetID NodeId) ([]Contact, error)`
- `FindValue(requester, recipient Contact, key NodeId) ([]byte, []Contact, error)`
- `Store(requester, recipient Contact, key NodeId, value []byte) error`

**SimNetwork** (Simulation Network):
- In-memory implementation for testing
- Thread-safe with mutex protection
- Maps NodeId to Node instances
- Allows testing DHT logic without network complexity
- All RPC calls are synchronous and immediate

### 5. **Contact Management** (`contact.go`)

- **Contact Structure**: Node ID, IP address, Port
- **ContactSorter**: Implements `sort.Interface` for sorting contacts by XOR distance
- **Utility Functions**: Printing, appending contacts

## Main Program (`main.go`)

The demo program demonstrates:

1. **Network Setup**: Creates a simulation network
2. **Node Creation**: 
   - Creates 3 initial test nodes ("test", "testt", "testtt")
   - Creates 10 additional nodes ("test 1" through "test 10")
3. **Network Joining**: All nodes join via the first node
4. **Lookup Demonstration**: Performs lookup for non-existent node "test 233"
5. **Output**: Prints closest nodes found with their XOR distances

## Testing (`kademlia_test.go`)

Comprehensive unit tests covering:

1. **Routing Table Update Tests**:
   - New contact when bucket not full
   - Existing contact moves to front (LRU)
   - New contact when bucket full + ping fails (replacement)
   - New contact when bucket full + ping succeeds (rejection)

2. **Test Utilities**:
   - `CreateContactForBucket()`: Creates contacts at specific bucket indices
   - Parallel test execution support

## Key Features

### ✅ Implemented

1. **Core Kademlia Protocol**:
   - XOR-based distance metric
   - K-bucket routing table (256 buckets)
   - Iterative node lookup (O(log n))
   - Node joining/bootstrap mechanism

2. **RPC Operations**:
   - Ping (node liveness check)
   - FindNode (node discovery)
   - FindValue (value retrieval)
   - Store (value storage)

3. **Thread Safety**:
   - Mutex protection on routing table operations
   - Thread-safe network simulation

4. **Testing Infrastructure**:
   - Comprehensive unit tests
   - Simulation network for isolated testing

### 🔄 Optimization Opportunities

1. **FindClosest Algorithm**: 
   - Current: O(n) iteration through all buckets
   - Proposed: Spiral lookup starting from target bucket (more efficient)

2. **Network Layer**:
   - Currently simulation-only
   - Could implement UDP-based real network communication

3. **Storage**:
   - Currently in-memory only
   - Could add persistence layer

## Technical Highlights

### Algorithm Complexity

- **Lookup**: O(log n) - logarithmic time complexity
- **Storage**: O(1) - constant time for storage operations
- **Routing Table Update**: O(k) - linear in bucket size

### Design Patterns

1. **Interface Segregation**: Network interface allows different implementations
2. **Strategy Pattern**: Ping function as parameter allows different ping strategies
3. **LRU Cache**: Efficient contact management using linked lists

### Concurrency

- Uses Go's `sync.RWMutex` for read-write locking
- Supports parallel test execution
- Thread-safe routing table operations

## Usage Example

```go
// Create simulation network
simNetwork := kademliadfs.NewSimNetwork()

// Create a node
node := kademliadfs.NewNode(
    kademliadfs.NewNodeId("my-node"),
    net.IPv4zero,
    10000,
    simNetwork)

// Register node
simNetwork.Register(node)

// Join network via bootstrap node
node.Join(bootstrapContact)

// Perform lookup
closestNodes := node.Lookup(targetNodeId)

// Store a value
node.HandleStore(requester, key, value)

// Find a value
value, contacts := node.HandleFindValue(requester, key)
```

## Dependencies

- **Go 1.25.0+**: Modern Go version
- **Standard Library Only**: No external dependencies
  - `crypto/sha256`: Node ID generation
  - `container/list`: LRU management
  - `math/big`: XOR distance calculations
  - `net`: IP address handling
  - `sync`: Thread synchronization

## Project Status

This is a **working implementation** of the Kademlia DHT protocol with:
- ✅ Core protocol implementation
- ✅ Simulation network for testing
- ✅ Comprehensive test suite
- ✅ Demo program demonstrating functionality

**Potential Extensions**:
- Real UDP network implementation
- Persistent storage
- Performance optimizations
- Additional test scenarios
- Performance benchmarking

## Academic Context

This appears to be part of a **CENG 415 Senior Design Project** focusing on:
- Decentralized file storage systems
- Kademlia DHT protocol research
- Performance analysis (lookup latency, resilience)
- Network simulation capabilities

The implementation provides a solid foundation for:
- Network size scalability testing
- Lookup latency measurement
- Resilience testing under node churn
- Performance analysis and graphing

