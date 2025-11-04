# Kademlia-based Decentralized File Store

**CENG 415 Senior Design Project & Seminar I**

**Team Members:**
- Hasan Çoban (290201008)
- Ece Tokgöz (290201070)
- Zeynep Rüzgar (290201079)

**Supervisor:** Dr. Burak Galip Aslan

## Project Description

Traditional cloud storage systems, such as Google Drive, concentrate data on servers controlled by a single entity. This centralized architecture presents severe and inherent risks, including censorship, privacy violations, and single points of failure that lead to service outages. This project proposes to design and implement a decentralized storage prototype as a direct response to these vulnerabilities, aiming for a more resilient, serverless, and censorship-resistant infrastructure.

## Key Features

- **Content-Addressable Storage**: Files are identified by their cryptographic hash (SHA-256), ensuring immutability and avoiding conflict-resolution mechanisms
- **Kademlia DHT Implementation**: Custom-built distributed hash table using XOR metric and k-buckets for O(log n) routing efficiency
- **Command-Line Interface**: Simple CLI for adding files and retrieving them from the network
- **Performance Metrics**: Built-in tracking of lookup latency and system resilience

## Architecture

```
kademlia-dfs/
├── cmd/kademlia-dfs/     # Main application entry point
├── internal/
│   ├── dht/              # Kademlia DHT implementation
│   │   ├── node.go      # NodeID and Node structures
│   │   ├── kbucket.go   # K-bucket implementation
│   │   ├── routing.go   # Routing table management
│   │   ├── lookup.go    # Lookup algorithm
│   │   └── dht.go       # Main DHT structure
│   ├── storage/         # Content-addressable storage
│   │   ├── content.go   # Content store implementation
│   │   ├── provider.go  # Provider record management
│   │   └── store.go     # Key-value store for DHT
│   ├── network/         # Network communication layer
│   │   └── transport.go # RPC transport implementation
│   ├── cli/             # Command-line interface
│   │   └── cmd.go       # CLI commands (add, get, bootstrap, metrics)
│   ├── config/          # Configuration management
│   │   └── config.go    # Config loading and saving
│   └── metrics/         # Performance metrics
│       ├── latency.go   # Lookup latency tracking
│       ├── resilience.go # Resilience metrics
│       └── collector.go # Metrics aggregation
└── go.mod               # Go module definition
```

## Installation

### Prerequisites

- Go 1.21 or later

### Build

```bash
go mod download
go build -o kademlia-dfs ./cmd/kademlia-dfs
```

## Usage

### Bootstrap a Node

Start a DHT node:

```bash
./kademlia-dfs bootstrap
```

### Add a File

Add a file to the decentralized storage:

```bash
./kademlia-dfs add /path/to/file.txt
```

This will:
1. Compute the SHA-256 hash of the file
2. Store the file locally
3. Publish a provider record to the DHT

### Retrieve a File

Retrieve a file by its hash:

```bash
./kademlia-dfs get <content-hash>
```

This will:
1. Query the DHT for the provider record
2. Locate the provider node
3. Download the file (when implemented)

### View Metrics

Display performance metrics:

```bash
./kademlia-dfs metrics
```

Export metrics to JSON:

```bash
METRICS_EXPORT=metrics.json ./kademlia-dfs metrics
```

## Configuration

Configuration is stored in `~/.kademlia-dfs/config.json`. Default configuration:

```json
{
  "ip": "127.0.0.1",
  "port": 4001,
  "storage_path": "~/.kademlia-dfs/storage",
  "k": 20,
  "alpha": 3,
  "bootstrap_nodes": []
}
```

## Implementation Details

### Kademlia DHT

- **NodeID**: 160-bit identifiers using XOR distance metric
- **K-buckets**: 160 buckets, each holding up to k=20 nodes
- **Lookup Algorithm**: Iterative lookup with α=3 parallel queries
- **Routing**: O(log n) lookup complexity

### Content-Addressable Storage

- Files are identified by SHA-256 hash
- Provider records stored in DHT mapping hash → node information
- Local storage in `~/.kademlia-dfs/storage/`

### Network Protocol

- TCP-based RPC protocol
- Message types: Ping, FindNode, FindValue, Store
- JSON message serialization

## Performance Metrics

The system tracks two key metrics:

1. **Lookup Latency**: How lookup time scales with network size
2. **Resilience**: System's ability to maintain data retrieval during network churn

## Development Status

- [x] DHT core implementation (node, k-buckets, routing)
- [x] Content-addressable storage
- [x] Network transport layer
- [x] CLI interface
- [x] Performance metrics collection
- [ ] File transfer protocol
- [ ] GUI interface (optional)
- [ ] Network simulation and testing
- [ ] Performance analysis and graphing

## License

This project is part of a senior design course at CENG.

## References

- Kademlia: A Peer-to-peer Information System Based on the XOR Metric (Maymounkov & Mazières, 2002)
- IPFS: InterPlanetary File System

