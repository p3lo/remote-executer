package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/creack/pty"
)

const (
	SERVER_PORT = ":7107"
	API_KEY     = "your-secret-key-12345"
)

type Config struct {
	Port    string
	APIKey  string
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

type Server struct {
	config   Config
	listener net.Listener
}

func NewServer(config Config) *Server {
	return &Server{
		config: config,
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	
	// Read initial command
	message, err := reader.ReadString('\n')
	if err != nil {
		log.Printf("Error reading from connection: %v\n", err)
		return
	}

	var cmd Command
	if err := json.Unmarshal([]byte(message), &cmd); err != nil {
		response := Response{Error: "Invalid command format"}
		json.NewEncoder(conn).Encode(response)
		return
	}

	// Validate API key
	if cmd.ApiKey != s.config.APIKey {
		response := Response{Error: "Invalid API key"}
		json.NewEncoder(conn).Encode(response)
		return
	}

	if cmd.Terminal {
		s.handleTerminalSession(conn)
	} else {
		s.handleSingleCommand(conn, cmd.Command)
	}
}

func (s *Server) handleTerminalSession(conn net.Conn) {
	// Start a shell
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}

	// Create command
	cmd := exec.Command(shell)
	
	// Start the command with a pty
	ptmx, err := pty.Start(cmd)
	if err != nil {
		log.Printf("Error starting pty: %v\n", err)
		return
	}
	defer ptmx.Close()

	// Create a channel to signal goroutine termination
	done := make(chan struct{})
	defer close(done)

	// Set up cleanup on signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		close(done)
		cmd.Process.Kill()
	}()

	// Handle input from client to PTY
	go func() {
		buffer := make([]byte, 1024)
		for {
			select {
			case <-done:
				return
			default:
				n, err := conn.Read(buffer)
				if err != nil {
					if err != io.EOF {
						log.Printf("Error reading from connection: %v\n", err)
					}
					cmd.Process.Kill()
					return
				}
				if _, err := ptmx.Write(buffer[:n]); err != nil {
					log.Printf("Error writing to pty: %v\n", err)
					cmd.Process.Kill()
					return
				}
			}
		}
	}()

	// Handle output from PTY to client
	go func() {
		buffer := make([]byte, 1024)
		for {
			select {
			case <-done:
				return
			default:
				n, err := ptmx.Read(buffer)
				if err != nil {
					if err != io.EOF {
						log.Printf("Error reading from pty: %v\n", err)
					}
					cmd.Process.Kill()
					return
				}
				if _, err := conn.Write(buffer[:n]); err != nil {
					log.Printf("Error writing to connection: %v\n", err)
					cmd.Process.Kill()
					return
				}
			}
		}
	}()

	// Wait for the command to finish
	if err := cmd.Wait(); err != nil {
		if err.Error() != "signal: killed" {
			log.Printf("Shell exited with error: %v\n", err)
		}
	}
}

func (s *Server) handleSingleCommand(conn net.Conn, command string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create command
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	
	// Get command output
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(conn, "Error executing command: %v\nOutput:\n%s", err, string(output))
		return
	}

	// Write output directly
	conn.Write(output)
}

func (s *Server) executeCommand(ctx context.Context, command string) (string, error) {
	// Basic command validation
	if strings.TrimSpace(command) == "" {
		return "", fmt.Errorf("empty command")
	}

	// Split the command string into command and arguments
	parts := strings.Fields(command)
	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	
	// Set command restrictions
	cmd.Env = []string{} // Restrict environment variables
	
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func (s *Server) Start() error {
	var err error
	s.listener, err = net.Listen("tcp", s.config.Port)
	if err != nil {
		return fmt.Errorf("failed to start server: %v", err)
	}

	log.Printf("Server listening on port %s\n", s.config.Port)

	// Handle graceful shutdown
	go s.handleSignals()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			log.Printf("Error accepting connection: %v\n", err)
			continue
		}

		go s.handleConnection(conn)
	}
}

func (s *Server) handleSignals() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	log.Println("Shutting down server...")
	s.listener.Close()
	os.Exit(0)
}

func main() {
	config := Config{
		Port:    SERVER_PORT,
		APIKey:  API_KEY,
	}

	server := NewServer(config)
	if err := server.Start(); err != nil {
		log.Fatal(err)
	}
}
