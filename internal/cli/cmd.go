package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/kademlia-dfs/kademlia-dfs/internal/config"
	"github.com/kademlia-dfs/kademlia-dfs/internal/dht"
	"github.com/kademlia-dfs/kademlia-dfs/internal/metrics"
	"github.com/kademlia-dfs/kademlia-dfs/internal/network"
	"github.com/kademlia-dfs/kademlia-dfs/internal/storage"
	"github.com/spf13/cobra"
)

var (
	cfg             *config.Config
	dhtInstance     *dht.DHT
	contentStore    *storage.ContentStore
	providerStore   *storage.ProviderStore
	transport       *network.Transport
	metricsCollector *metrics.Collector
)

var rootCmd = &cobra.Command{
	Use:   "kademlia-dfs",
	Short: "Kademlia-based Decentralized File Store",
	Long: `A decentralized file storage system based on the Kademlia DHT protocol.
This system provides content-addressable storage with O(log n) lookup performance.`,
}

var addCmd = &cobra.Command{
	Use:   "add [file]",
	Short: "Add a file to the DHT",
	Long:  "Adds a file to the decentralized storage system and publishes its hash and provider record to the DHT.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return addFile(args[0])
	},
}

var getCmd = &cobra.Command{
	Use:   "get [hash]",
	Short: "Retrieve a file from the DHT",
	Long:  "Queries the DHT for a content hash, finds the provider, and downloads the file.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return getFile(args[0])
	},
}

var bootstrapCmd = &cobra.Command{
	Use:   "bootstrap",
	Short: "Bootstrap the DHT node",
	Long:  "Initializes and starts the DHT node with network connectivity.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return bootstrap()
	},
}

var metricsCmd = &cobra.Command{
	Use:   "metrics",
	Short: "Display performance metrics",
	Long:  "Shows latency and resilience metrics for the DHT system.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return showMetrics()
	},
}

// Execute runs the CLI
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(getCmd)
	rootCmd.AddCommand(bootstrapCmd)
	rootCmd.AddCommand(metricsCmd)
}

// initialize initializes the DHT and storage components
func initialize() error {
	var err error

	// Load configuration
	cfg, err = config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Generate or load node ID
	nodeID := dht.NewNodeID()
	if cfg.NodeID != "" {
		nodeID, err = dht.NodeIDFromString(cfg.NodeID)
		if err != nil {
			return fmt.Errorf("invalid node ID: %w", err)
		}
	}

	// Create DHT instance
	dhtInstance = dht.NewDHT(nodeID, cfg.IP, cfg.Port)

	// Create content store
	contentStore, err = storage.NewContentStore(cfg.StoragePath)
	if err != nil {
		return fmt.Errorf("failed to create content store: %w", err)
	}

	// Create provider store
	providerStore = storage.NewProviderStore()

	// Create KV store for DHT
	kvStore := storage.NewKVStore()
	dhtInstance.SetStorage(kvStore)

	// Create metrics collector
	metricsCollector = metrics.NewCollector()

	// Create and start transport
	transport = network.NewTransport(cfg.Port, dhtInstance.GetSelfNode(), dhtInstance)
	transport.SetContentStore(contentStore)
	
	if err := transport.Start(); err != nil {
		return fmt.Errorf("failed to start transport: %w", err)
	}

	// Set network on DHT
	dhtInstance.SetNetwork(transport)

	return nil
}

// addFile adds a file to the storage system
func addFile(filePath string) error {
	if err := initialize(); err != nil {
		return err
	}
	defer transport.Stop()

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("file not found: %s", filePath)
	}

	// Add file to content store
	hash, err := contentStore.AddFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to add file: %w", err)
	}

	fmt.Printf("File added successfully. Hash: %s\n", hash.String())

	// Convert ContentHash to NodeID for DHT storage
	var key dht.NodeID
	copy(key[:], hash[:])

	// Create provider record
	selfNode := dhtInstance.GetSelfNode()
	providerStore.AddProvider(hash.String(), selfNode)

	record, err := storage.SerializeProviderRecord(&storage.ProviderRecord{
		Hash:      hash.String(),
		IP:        selfNode.IP,
		Port:      selfNode.Port,
		NodeID:    selfNode.ID.String(),
		Timestamp: time.Now().Unix(),
	})
	if err != nil {
		return fmt.Errorf("failed to serialize provider record: %w", err)
	}

	// Store provider record in DHT
	startTime := time.Now()
	if err := dhtInstance.Store(key, record); err != nil {
		return fmt.Errorf("failed to store provider record: %w", err)
	}
	duration := time.Since(startTime)
	metricsCollector.RecordLookup(duration)

	fmt.Printf("Provider record published to DHT\n")
	return nil
}

// getFile retrieves a file from the storage system
func getFile(hashStr string) error {
	if err := initialize(); err != nil {
		return err
	}
	defer transport.Stop()

	// Parse hash
	hash, err := storage.ContentHashFromString(hashStr)
	if err != nil {
		return fmt.Errorf("invalid hash: %w", err)
	}

	// Check local storage first
	if contentStore.HasFile(hash) {
		data, err := contentStore.GetFile(hash)
		if err == nil {
			fmt.Printf("File found locally\n")
			fmt.Printf("Content length: %d bytes\n", len(data))
			metricsCollector.RecordRetrieval(true)
			return nil
		}
	}

	// Convert ContentHash to NodeID for DHT lookup
	var key dht.NodeID
	copy(key[:], hash[:])

	// Lookup provider record in DHT
	startTime := time.Now()
	value, err := dhtInstance.GetValue(key)
	duration := time.Since(startTime)
	metricsCollector.RecordLookup(duration)

	if err != nil {
		fmt.Printf("Failed to find provider: %v\n", err)
		metricsCollector.RecordRetrieval(false)
		return err
	}

	// Deserialize provider record
	providerRecord, err := storage.DeserializeProviderRecord(value)
	if err != nil {
		metricsCollector.RecordRetrieval(false)
		return fmt.Errorf("failed to deserialize provider record: %w", err)
	}

	fmt.Printf("Found provider: %s:%d\n", providerRecord.IP, providerRecord.Port)

	// TODO: Download file from provider
	// This would require implementing file transfer protocol
	fmt.Printf("File retrieval from provider not yet implemented\n")
	
	metricsCollector.RecordRetrieval(true)
	return nil
}

// bootstrap bootstraps the DHT node
func bootstrap() error {
	if err := initialize(); err != nil {
		return err
	}
	defer transport.Stop()

	// Bootstrap with configured nodes
	if len(cfg.BootstrapNodes) > 0 {
		bootstrapNodes := make([]*dht.Node, 0, len(cfg.BootstrapNodes))
		for _, addr := range cfg.BootstrapNodes {
			// Parse bootstrap node address (format: "ip:port" or "nodeid:ip:port")
			// For now, create a placeholder node
			nodeID := dht.NewNodeID()
			node := dht.NewNode(nodeID, "127.0.0.1", 4001) // Default
			bootstrapNodes = append(bootstrapNodes, node)
		}

		if err := dhtInstance.Bootstrap(bootstrapNodes); err != nil {
			return fmt.Errorf("bootstrap failed: %w", err)
		}
	}

	fmt.Printf("Node ID: %s\n", dhtInstance.GetNodeID().String())
	fmt.Printf("Listening on %s:%d\n", cfg.IP, cfg.Port)
	fmt.Printf("Routing table size: %d nodes\n", dhtInstance.GetRoutingTable().GetNodeCount())
	fmt.Printf("DHT node bootstrapped successfully\n")

	// Keep running
	select {}
}

// showMetrics displays performance metrics
func showMetrics() error {
	if err := initialize(); err != nil {
		return err
	}
	defer transport.Stop()

	summary := metricsCollector.GetSummary()

	fmt.Println("=== Performance Metrics ===")
	fmt.Printf("Uptime: %s\n", summary.Uptime)
	fmt.Printf("Network Size: %d nodes\n", summary.NetworkSize)
	
	fmt.Println("\n=== Latency Metrics ===")
	fmt.Printf("Total Lookups: %d\n", summary.LatencyStats.Count)
	if summary.LatencyStats.Count > 0 {
		fmt.Printf("Average Latency: %v\n", summary.LatencyStats.Average)
		fmt.Printf("Min Latency: %v\n", summary.LatencyStats.Min)
		fmt.Printf("Max Latency: %v\n", summary.LatencyStats.Max)
	}

	fmt.Println("\n=== Resilience Metrics ===")
	fmt.Printf("Total Retrievals: %d\n", summary.ResilienceStats.TotalRetrievals)
	fmt.Printf("Successful: %d\n", summary.ResilienceStats.SuccessfulRetrievals)
	fmt.Printf("Failed: %d\n", summary.ResilienceStats.FailedRetrievals)
	fmt.Printf("Success Rate: %.2f%%\n", summary.ResilienceStats.SuccessRate)
	fmt.Printf("Node Failures: %d\n", summary.ResilienceStats.NodeFailures)
	fmt.Printf("Churn Events: %d\n", summary.ResilienceStats.ChurnEvents)

	// Export to JSON if requested
	exportPath := os.Getenv("METRICS_EXPORT")
	if exportPath != "" {
		data, err := json.MarshalIndent(summary, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal metrics: %w", err)
		}
		if err := os.WriteFile(exportPath, data, 0644); err != nil {
			return fmt.Errorf("failed to write metrics file: %w", err)
		}
		fmt.Printf("\nMetrics exported to: %s\n", exportPath)
	}

	return nil
}

