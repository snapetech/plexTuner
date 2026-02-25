package hdhomerun

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
)

// Config holds HDHomeRun network mode configuration
type Config struct {
	Enabled      bool
	DeviceID     uint32
	TunerCount   int
	DiscoverPort int
	ControlPort  int
	BaseURL      string
	FriendlyName string
}

// StreamFunc is a function that returns a stream for a channel
type StreamFunc func(ctx context.Context, channelID string) (io.ReadCloser, error)

// Server is the main HDHomeRun network server
type Server struct {
	config     *Config
	device     *Device
	control    *ControlServer
	discover   *DiscoverServer
	streamFunc StreamFunc
}

// NewServer creates a new HDHomeRun network server
func NewServer(config *Config, streamFunc StreamFunc) (*Server, error) {
	if config.TunerCount <= 0 {
		config.TunerCount = 2
	}

	device := CreateDefaultDevice(config.DeviceID, config.TunerCount, config.BaseURL)
	// Override friendly name if provided in config
	if config.FriendlyName != "" {
		device.FriendlyName = config.FriendlyName
	}

	return &Server{
		config:     config,
		device:     device,
		streamFunc: streamFunc,
	}, nil
}

// Run starts the HDHomeRun server (blocking)
func (s *Server) Run(ctx context.Context) error {
	if !s.config.Enabled {
		log.Printf("hdhomerun: network mode disabled")
		return nil
	}

	log.Printf("hdhomerun: starting network mode (device_id=0x%08x, tuners=%d)",
		s.device.DeviceID, s.device.TunerCount)

	// Start UDP discovery server
	discover, err := NewDiscoverServer(s.device, s.config.DiscoverPort)
	if err != nil {
		return fmt.Errorf("start discovery: %w", err)
	}
	s.discover = discover

	// Start TCP control server with stream function
	control := NewControlServer(s.device, s.device.TunerCount, s.config.BaseURL, s.streamFunc)

	// Create TCP listener
	controlAddr := &net.TCPAddr{
		Port: s.config.ControlPort,
		IP:   net.IPv4zero,
	}
	listener, err := net.ListenTCP("tcp4", controlAddr)
	if err != nil {
		discover.Close()
		return fmt.Errorf("listen TCP: %w", err)
	}
	s.control = control

	// Handle shutdown
	go func() {
		<-ctx.Done()
		log.Printf("hdhomerun: shutting down")
		listener.Close()
		discover.Close()
	}()

	// Start TCP server in goroutine
	go func() {
		if err := control.Serve(listener); err != nil {
			log.Printf("hdhomerun: control server error: %v", err)
		}
	}()

	// Run discovery server (blocking)
	if err := discover.Run(); err != nil {
		log.Printf("hdhomerun: discovery error: %v", err)
	}

	return nil
}

// GetDevice returns the device configuration
func (s *Server) GetDevice() *Device {
	return s.device
}

// Example main function showing usage
func Example() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	config := &Config{
		Enabled:    false,
		DeviceID:   0x12345678,
		TunerCount: 2,
		BaseURL:    "http://192.168.1.100:5004",
	}

	// Example stream function - would connect to existing gateway
	streamFunc := func(ctx context.Context, channelID string) (io.ReadCloser, error) {
		return nil, fmt.Errorf("not implemented")
	}

	server, err := NewServer(config, streamFunc)
	if err != nil {
		log.Fatalf("create server: %v", err)
	}

	if err := server.Run(ctx); err != nil {
		log.Fatalf("run server: %v", err)
	}
}
