package hdhomerun

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
)

// Config holds HDHomeRun network mode configuration
type Config struct {
	Enabled        bool
	DeviceID       uint32
	TunerCount     int
	DiscoverPort   int
	ControlPort    int
	BaseURL        string
}

// Server is the main HDHomeRun network server
type Server struct {
	config  *Config
	device  *Device
	control *ControlServer
	discover *DiscoverServer
}

// NewServer creates a new HDHomeRun network server
func NewServer(config *Config) (*Server, error) {
	if config.TunerCount <= 0 {
		config.TunerCount = 2
	}

	device := CreateDefaultDevice(config.DeviceID, config.TunerCount, config.BaseURL)

	return &Server{
		config: config,
		device: device,
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

	// Start TCP control server
	control := NewControlServer(s.device, s.device.TunerCount, s.config.BaseURL)

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

// LoadConfig loads HDHomeRun configuration from environment
func LoadConfig() *Config {
	return &Config{
		Enabled:      getEnvBool("PLEX_TUNER_HDHR_NETWORK_MODE", false),
		DeviceID:     getEnvUint32("PLEX_TUNER_HDHR_DEVICE_ID", 0x12345678),
		TunerCount:   getEnvInt("PLEX_TUNER_HDHR_TUNER_COUNT", 2),
		DiscoverPort: getEnvInt("PLEX_TUNER_HDHR_DISCOVER_PORT", 65001),
		ControlPort:  getEnvInt("PLEX_TUNER_HDHR_CONTROL_PORT", 65001),
		BaseURL:      os.Getenv("PLEX_TUNER_BASE_URL"),
	}
}

func getEnvBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

func getEnvInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		return def
	}
	return n
}

func getEnvUint32(key string, def uint32) uint32 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	var n uint32
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		return def
	}
	return n
}

// Example main function showing usage
func Example() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	config := LoadConfig()
	config.BaseURL = "http://192.168.1.100:5004"

	server, err := NewServer(config)
	if err != nil {
		log.Fatalf("create server: %v", err)
	}

	if err := server.Run(ctx); err != nil {
		log.Fatalf("run server: %v", err)
	}
}
