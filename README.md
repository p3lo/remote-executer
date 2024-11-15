# Remote Command Executor

A cross-platform remote command execution tool with secure TCP communication and interactive terminal capabilities. Available in both Go and Rust implementations.

## Features

- TCP communication on port 7107
- API key authentication
- Two execution modes:
  - Single command execution
  - Interactive terminal session
- Supports PTY (Pseudo-Terminal) for interactive sessions
- Cross-platform support (Linux, macOS, Windows)

## Go Implementation

### Building

```bash
# Build server
cd server
go build

# Build client
cd client
go build
```

### Usage

1. Start the server:
```bash
./remote-executor-server
```

2. Use the client:
```bash
# For single command execution
./remote-executor-client -s localhost -c "ls -la"

# For interactive terminal session
./remote-executor-client -s localhost -t
```

## Security Note

The default API key is "your-secret-key-12345". For production use, please change this to a secure value.

## License

MIT License
