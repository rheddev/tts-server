# TTS Server

A Go-based WebSocket server for handling real-time text-to-speech message broadcasting.

## Features

- WebSocket-based real-time message broadcasting
- RESTful API endpoints for message management
- Basic authentication for admin access
- PostgreSQL database integration
- Configurable through environment variables
- CORS support for frontend integration
- Graceful shutdown handling

## Prerequisites

- Go 1.16 or higher
- PostgreSQL database
- Environment variables (see Configuration section)

## Installation

1. Clone the repository:
```bash
git clone https://github.com/rheddev/tts-server.git
cd tts-server
```

2. Install dependencies:
```bash
go mod download
```

3. Set up environment variables (see Configuration section)

4. Build the project:
```bash
go build -o tts-server
```

## Configuration

Create a `.env` file in the project root with the following variables:

```env
PORT=8080
FRONTEND_URL=http://localhost:5173
ADMIN_USERNAME=admin
ADMIN_PASSWORD=your-secure-password
READ_TIMEOUT=5
WRITE_TIMEOUT=10
SHUTDOWN_TIMEOUT=30
```

## API Endpoints

### WebSocket Endpoints
- `GET /ws/listen` - WebSocket connection for receiving messages
- `POST /ws/send` - Endpoint for sending messages

### REST Endpoints
- `GET /ping` - Health check endpoint
- `GET /messages` - Get messages (requires admin authentication)
  - Query parameters:
    - `from`: Start time (RFC3339 format)
    - `to`: End time (RFC3339 format)

## Running the Server

```bash
./tts-server
```

The server will start on the configured port (default: 8080).

## Development

To run the server in development mode:

```bash
go run src/main.go
```