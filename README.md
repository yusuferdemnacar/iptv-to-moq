# IPTV to Moq

This project converts IPTV channel lists into Moq format. It is written in Go and provides a command-line interface for processing IPTV channel lists.

## Requirements

- Go 1.22 or later
- ffmpeg
- ffplay

## Installation

1. **Clone the repository:**

    ```
    git clone https://github.com/yourusername/iptv-to-moq.git
    cd iptv-to-moq
    ```

1. **Install dependencies:**

    Ensure you have Go installed. Then, run:

    ```
    go mod tidy
    ```

1. **Build the project:**

    ```sh
    go build -o iptv-to-moq
    ```

## Usage

- **Run the server:**

    ```
    ./iptv-to-moq --cert <path-to-certificate> \
                  --key <path-to-key> \
                  --addr <ip:port-to-listen-on> \
                  --quic \
                  --server \
                  --addr <ip:port> 
    ```

    - `--cert`: Path to the certificate file.
        - Default: `localhost.pem`
    - `--key`: Path to the key file.
        - Default: `localhost-key.pem`
    - `--addr`: IP address and port to listen on.
        - Default: `localhost:8080`
    - `--quic`: Whether to use raw QUIC or WebTransport as transport. Presence of this sets raw QUIC mode.
        - Default: `false`
    - `--server`: To run the server. Presence of this sets the server mode.
        - Default: `false`
    
- **Run the client:**

    ```
    ./iptv-to-moq --addr <ip:port-of-server> \
                  --quic \
                  --iptv-addr <iptv-stream-URL> --output <path-to-moq-file>
                  --cli
    ```

    - `--addr`: IP address and port of the server.
        - Default: `localhost:8080`
    - `--quic`: Whether to use raw QUIC or WebTransport as transport. Presence of this sets raw QUIC mode.
        - Default: `false`
    - `--iptv-addr`: URL of the IPTV stream to be asked of the server to convert.
        - Default: No default value. Mandatory.
    - `--cli`: Whether to run the client in CLI mode. Presence of this sets the CLI mode.
        - Default: `false`