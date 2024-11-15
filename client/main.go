package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"
)

const (
	DEFAULT_PORT = ":7107"
	API_KEY      = "your-secret-key-12345"
)

type Config struct {
	APIKey          string
	ConnectTimeout  time.Duration
	RequestTimeout  time.Duration
	RetryAttempts   int
	RetryDelay      time.Duration
}

type Command struct {
	ApiKey   string `json:"api_key"`
	Command  string `json:"command"`
	Terminal bool   `json:"terminal"`
}

type Response struct {
	Output string `json:"output"`
	Error  string `json:"error"`
}

type Client struct {
	config Config
}

func NewClient(config Config) *Client {
	return &Client{
		config: config,
	}
}

func (c *Client) startTerminalSession(server string) error {
	// Add default port if not specified
	if !strings.Contains(server, ":") {
		server = server + DEFAULT_PORT
	}

	// Connect to the server
	conn, err := net.DialTimeout("tcp", server, c.config.ConnectTimeout)
	if err != nil {
		return fmt.Errorf("error connecting: %v", err)
	}
	defer conn.Close()

	// Send initial terminal command
	cmd := Command{
		ApiKey:   c.config.APIKey,
		Terminal: true,
	}

	cmdJson, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("error encoding command: %v", err)
	}

	if _, err := fmt.Fprintf(conn, "%s\n", cmdJson); err != nil {
		return fmt.Errorf("error sending command: %v", err)
	}

	// Get current terminal size
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return fmt.Errorf("not running in a terminal")
	}

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("error setting raw terminal: %v", err)
	}
	defer term.Restore(fd, oldState)

	// Create a channel for cleanup
	done := make(chan struct{})
	defer close(done)

	// Handle Ctrl+C and other signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		close(done)
		term.Restore(fd, oldState)
		fmt.Println("\r\nTerminal session ended.")
		os.Exit(0)
	}()

	// Start the terminal session
	fmt.Println("\r\nTerminal session started. Press Ctrl+C to exit.\r")

	// Handle input from terminal
	go func() {
		buffer := make([]byte, 1024)
		for {
			select {
			case <-done:
				return
			default:
				n, err := os.Stdin.Read(buffer)
				if err != nil {
					if err != io.EOF {
						log.Printf("Error reading from stdin: %v\n", err)
					}
					return
				}
				if _, err := conn.Write(buffer[:n]); err != nil {
					log.Printf("Error writing to connection: %v\n", err)
					return
				}
			}
		}
	}()

	// Handle output from server
	buffer := make([]byte, 1024)
	for {
		select {
		case <-done:
			return nil
		default:
			n, err := conn.Read(buffer)
			if err != nil {
				if err != io.EOF {
					return fmt.Errorf("connection closed: %v", err)
				}
				return nil
			}
			if _, err := os.Stdout.Write(buffer[:n]); err != nil {
				return fmt.Errorf("error writing to stdout: %v", err)
			}
		}
	}
}

func (c *Client) executeCommand(server, command string) error {
	// Add default port if not specified
	if !strings.Contains(server, ":") {
		server = server + DEFAULT_PORT
	}

	// Connect to the server
	conn, err := net.DialTimeout("tcp", server, c.config.ConnectTimeout)
	if err != nil {
		return fmt.Errorf("error connecting to %s: %v", server, err)
	}
	defer conn.Close()

	// Send command
	cmd := Command{
		ApiKey:  c.config.APIKey,
		Command: command,
	}

	cmdJson, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("error encoding command: %v", err)
	}

	if _, err := fmt.Fprintf(conn, "%s\n", cmdJson); err != nil {
		return fmt.Errorf("error sending command: %v", err)
	}

	fmt.Printf("\nOutput from %s:\n", server)

	// Read and print the raw output
	_, err = io.Copy(os.Stdout, conn)
	if err != nil {
		return fmt.Errorf("error reading response: %v", err)
	}

	return nil
}

func main() {
	// Parse command line flags
	terminalMode := flag.Bool("terminal", false, "Start an interactive terminal session")
	commandMode := flag.String("command", "", "Execute a command on the remote server")
	flag.Parse()

	// Load configuration
	config := Config{
		APIKey:          API_KEY,
		ConnectTimeout:  5 * time.Second,
		RequestTimeout:  30 * time.Second,
		RetryAttempts:   3,
		RetryDelay:      time.Second,
	}

	client := NewClient(config)

	if *terminalMode {
		// Check if server argument is provided
		if flag.NArg() != 1 {
			fmt.Println("Usage for terminal mode: client -terminal <server>")
			fmt.Println("Example: client -terminal 127.0.0.1")
			os.Exit(1)
		}
		
		server := flag.Arg(0)
		fmt.Printf("Starting terminal session on %s...\n", server)
		if err := client.startTerminalSession(server); err != nil {
			log.Fatalf("Terminal session failed: %v", err)
		}
	} else if *commandMode != "" {
		// Check if server argument is provided
		if flag.NArg() != 1 {
			fmt.Println("Usage for command mode: client -command \"<command>\" <server>")
			fmt.Println("Example: client -command \"ls -la\" 127.0.0.1")
			os.Exit(1)
		}

		server := flag.Arg(0)
		if err := client.executeCommand(server, *commandMode); err != nil {
			log.Fatalf("Command execution failed: %v", err)
		}
	} else {
		fmt.Println("Usage:")
		fmt.Println("  For terminal mode: client -terminal <server>")
		fmt.Println("  For command mode: client -command \"<command>\" <server>")
		fmt.Println("\nExamples:")
		fmt.Println("  client -terminal 127.0.0.1")
		fmt.Println("  client -command \"ls -la\" 127.0.0.1")
		os.Exit(1)
	}
}
