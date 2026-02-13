package main

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"flag"
	"fmt"
	"log"
	"net"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"

	kademliadfs "github.com/hasocobo/kademlia-dfs/kademlia"
	"github.com/hasocobo/kademlia-dfs/runtime"
	"github.com/hasocobo/kademlia-dfs/scheduler"
	"github.com/prometheus/client_golang/prometheus"
)

//go:embed .binaries/add.wasm
var wasmAdd []byte

//go:embed .binaries/subtract.wasm
var wasmSubtract []byte

type Config struct {
	IP            net.IP
	Port          int
	BootstrapIP   net.IP
	BootstrapPort int
	IsBootstrap   bool
}

func main() {
	cfg := parseFlags()

	err := run(context.Background(), cfg)
	if err == nil {
		os.Exit(0)
	}
	os.Exit(1)
}

func run(ctx context.Context, cfg Config) error {
	udpPort := cfg.Port
	udpIP := cfg.IP
	nodeID := kademliadfs.NewRandomId()
	isBootstrapNode := cfg.IsBootstrap

	if isBootstrapNode {
		udpPort = cfg.BootstrapPort
		udpIP = cfg.BootstrapIP
		nodeID = kademliadfs.NodeId{}
	}

	errChan := make(chan error, 2)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// quicNetwork, err := kademliadfs.NewUDPNetwork(udpIP, udpPort)
	quicNetwork, err := kademliadfs.NewQUICNetwork(udpIP, udpPort)
	if err != nil {
		log.Print(err)
		return fmt.Errorf("error starting new udp network: %v", err)
	}

	go func() { errChan <- quicNetwork.Listen(ctx) }()
	//	if err := quicNetwork.SendSTUNRequest(ctx); err != nil {
	//		log.Print(err)
	//	}

	taskRuntime := runtime.WasmRuntime{}

	var node *kademliadfs.Node
	if quicNetwork.PublicAddr != nil {
		node = kademliadfs.NewNode(ctx, nodeID, quicNetwork.PublicAddr.IP, quicNetwork.PublicAddr.Port, quicNetwork)
	} else {
		localIP, err := kademliadfs.GetOutboundIP(ctx)
		if err != nil {
			log.Print(err)
			return fmt.Errorf("error getting outbound ip: %v", err)
		}
		log.Println("failed determining public address, switching to local address")
		log.Println(localIP)

		// makes node id the same for same ip + port always. temporary for testing restarts.
		sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", localIP.String(), udpPort)))

		nodeID = kademliadfs.NodeId(sum)

		node = kademliadfs.NewNode(ctx, nodeID, localIP, udpPort, quicNetwork)
	}

	parser := scheduler.YAMLParser{}
	reg := prometheus.NewRegistry()
	prometheusStats := scheduler.NewPrometheusStats(reg)
	scheduler := scheduler.NewScheduler(node, taskRuntime, quicNetwork, prometheusStats)

	quicNetwork.SetDHTHandler(node)
	quicNetwork.SetTaskHandler(scheduler)

	if !isBootstrapNode {
		bootstrapContact := kademliadfs.Contact{
			IP:   cfg.BootstrapIP,
			Port: cfg.BootstrapPort,
			ID:   kademliadfs.NodeId{},
		}
		if err := node.Join(ctx, bootstrapContact); err != nil {
			log.Printf("failed to join network: %v", err)
			return fmt.Errorf("failed to join network: %v", err)
		}
	}

	server := NewServer(scheduler, udpPort+1000, parser, reg)
	go func() {
		if err := server.ServeHTTP(ctx); err != nil {
			log.Printf("http server error: %v", err)
			errChan <- err
		}
	}()

	scheduler.Start(ctx)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)

	select {
	case <-quit:
		log.Println("signal received, shutting down...")
		cancel()
	case err := <-errChan:
		log.Printf("error in listener: %v, shutting down...", err)
		cancel()
		return err
	}
	return nil
}

func parseFlags() Config {
	ipPtr := flag.String("ip", "0.0.0.0", "usage: -ip=0.0.0.0")
	portPtr := flag.Int("port", 9999, "-port=9999")
	bootstrapNodeIpPtr := flag.String("bootstrap-ip", "0.0.0.0", "usage: -ip=0.0.0.0")
	bootstrapNodePortPtr := flag.Int("bootstrap-port", 9000, "-bootstrap-port=9000")
	isBootstrapNodePtr := flag.Bool("is-bootstrap", false, "is-bootstrap=false")
	flag.Parse()

	return Config{net.ParseIP(*ipPtr), *portPtr, net.ParseIP(*bootstrapNodeIpPtr), *bootstrapNodePortPtr, *isBootstrapNodePtr}
}
