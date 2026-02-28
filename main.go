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
	"strings"
	"syscall"

	kademliadfs "github.com/hasocobo/kademlia-dfs/kademlia"
	"github.com/hasocobo/kademlia-dfs/runtime"
	"github.com/hasocobo/kademlia-dfs/scheduler"
	"github.com/prometheus/client_golang/prometheus"
)

type Config struct {
	IP            net.IP
	Port          int
	BootstrapIP   net.IP
	BootstrapPort int
	IsBootstrap   bool

	MountDir string

	Tags               []string
	DeviceCapabilities []string
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

	taskRuntime := runtime.NewWasmRuntime(runtime.WasmConfig{
		MountDir:           cfg.MountDir,
		Tags:               cfg.Tags,
		DeviceCapabilities: cfg.DeviceCapabilities,
	})

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
	sched := scheduler.NewScheduler(node, taskRuntime, quicNetwork, prometheusStats)
	worker := scheduler.NewWorker(taskRuntime, quicNetwork, cfg.MountDir, cfg.DeviceCapabilities, cfg.Tags)
	worker.RegisterTopicPublisher(node)
	taskHandler := scheduler.NewTaskHandler(sched, worker)

	quicNetwork.SetDHTHandler(node)
	quicNetwork.SetTaskHandler(taskHandler)

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

	server := NewServer(sched, udpPort+1000, parser, reg)
	go func() {
		if err := server.ServeHTTP(ctx); err != nil {
			log.Printf("http server error: %v", err)
			errChan <- err
		}
	}()

	if isBootstrapNode {
		sched.Start(ctx)
	} else {
		worker.Start(ctx)
	}

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
	bootstrapNodeIpPtr := flag.String("bootstrap-ip", "0.0.0.0", "usage: -bootstrap-ip=0.0.0.0")
	bootstrapNodePortPtr := flag.Int("bootstrap-port", 9000, "-bootstrap-port=9000")
	isBootstrapNodePtr := flag.Bool("is-bootstrap", false, "is-bootstrap=false")

	tagsPtr := flag.String("tags", "", "comma separated tags: -tags=cctv_camera,thermostat")
	deviceCapsPtr := flag.String("device-capabilities", "", "comma separated capabilities")
	mountDirPtr := flag.String("mountdir", "", "mount directory for task runtime")

	flag.Parse()

	return Config{
		IP:                 net.ParseIP(*ipPtr),
		Port:               *portPtr,
		BootstrapIP:        net.ParseIP(*bootstrapNodeIpPtr),
		BootstrapPort:      *bootstrapNodePortPtr,
		IsBootstrap:        *isBootstrapNodePtr,
		Tags:               splitComma(*tagsPtr),
		DeviceCapabilities: splitComma(*deviceCapsPtr),
		MountDir:           *mountDirPtr,
	}
}

func splitComma(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}
