package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"math/big"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/manifoldco/promptui"
	"github.com/mengelbart/moqtransport"
)

var (
	serverStarted bool
	serverWg      sync.WaitGroup
	playlist      map[string]string // Map to store channel name and URL
	playing       []string          // List to store currently playing channels
	mu            sync.Mutex        // Mutex to protect the playing list
)

func main() {
	certFile := flag.String("cert", "localhost.pem", "TLS certificate file")
	keyFile := flag.String("key", "localhost-key.pem", "TLS key file")
	addr := flag.String("addr", "localhost:8080", "listen address")
	runAsServer := flag.Bool("server", false, "if set, run as server otherwise client")
	iptvAddr := flag.String("iptv-addr", "", "iptv stream address")
	cliMode := flag.Bool("cli", false, "run in interactive CLI mode")
	flag.Parse()

	// Open the null device as a file
	nullFile, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		fmt.Printf("failed to open null device: %v", err)
	}
	defer nullFile.Close()

	moqtransport.SetLogHandler(slog.NewJSONHandler(nullFile, &slog.HandlerOptions{}))
	// moqtransport.SetLogHandler(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{}))

	if *runAsServer {
		if err := runServer(*addr, *certFile, *keyFile); err != nil {
			fmt.Printf("failed to run server: %v", err)
		}
		return
	}

	if *cliMode {
		runCLI(addr)
		return
	}

	if err := runClient(*addr, *iptvAddr); err != nil {
		log.Panicf("failed to run client: %v", err)
	}
	log.Println("bye")
}

func runCLI(addr *string) {
	fmt.Println("Welcome to the IPTV CLI")
	for {
		prompt := promptui.Select{
			Label: "Select Action",
			Items: []string{"Upload IPTV Playlist Link", "Upload IPTV Playlist File", "Play Specific Channel", "Exit"},
		}

		_, result, err := prompt.Run()
		if err != nil {
			fmt.Printf("Prompt failed %v\n", err)
			return
		}

		switch result {
		case "Upload IPTV Playlist Link":
			uploadPlaylistAndPlayChannel(addr)
		case "Upload IPTV Playlist File":
			uploadPlaylistFileAndPlayChannel(addr)
		case "Play Specific Channel":
			playSpecificChannel(addr)
		case "Exit":
			if serverStarted {
				serverWg.Wait()
			}
			return
		}
	}
}

func playSpecificChannel(addr *string) {
	fmt.Print("Enter Channel URL: ")
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		channelURL := scanner.Text()
		finalURL := getFinalChannelURL(channelURL)
		channelName := getChannelNameFromURL(channelURL)
		fmt.Printf("Playing Channel: %s\n", channelName)
		mu.Lock()
		playing = append(playing, channelName)
		mu.Unlock()
		go func() {
			if err := runClient(*addr, finalURL); err != nil {
				log.Panicf("failed to run client: %v", err)
			}
		}()
	}
}

func uploadPlaylistAndPlayChannel(addr *string) {
	for {
		fmt.Print("Enter IPTV playlist link: ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			playlistLink := scanner.Text()
			fmt.Printf("Uploaded playlist link: %s\n", playlistLink)
			// Fetch and parse the playlist
			fetchAndParsePlaylist(playlistLink)
		}

		if playlist == nil || len(playlist) == 0 {
			fmt.Println("Failed to fetch or parse the playlist. Please try again.")
			return
		}

		if !selectAndPlayChannel(addr) {
			break
		}
	}
}

func uploadPlaylistFileAndPlayChannel(addr *string) {
	for {
		fmt.Print("Enter path to the playlist file: ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			filePath := scanner.Text()
			fmt.Printf("Uploading playlist file: %s\n", filePath)
			// Fetch and parse the playlist
			fetchAndParsePlaylistFile(filePath)
		}

		if playlist == nil || len(playlist) == 0 {
			fmt.Println("Failed to fetch or parse the playlist. Please try again.")
			return
		}

		if !selectAndPlayChannel(addr) {
			break
		}
	}
}

func fetchAndParsePlaylist(url string) {
	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("failed to fetch playlist: %v", err)
	}
	defer resp.Body.Close()

	playlist = make(map[string]string)

	scanner := bufio.NewScanner(resp.Body)
	var channelName string

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#EXTINF:") {
			channelName = strings.SplitN(line, ",", 2)[1]
		} else if strings.HasPrefix(line, "http") {
			playlist[channelName] = line
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("failed to parse playlist: %v", err)
	}

	fmt.Println("Playlist uploaded successfully.")
}

func fetchAndParsePlaylistFile(filePath string) {
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Printf("failed to open playlist file: %v", err)
	}
	defer file.Close()

	playlist = make(map[string]string)

	scanner := bufio.NewScanner(file)
	var channelName string

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#EXTINF:") {
			channelName = strings.SplitN(line, ",", 2)[1]
		} else if strings.HasPrefix(line, "http") {
			playlist[channelName] = line
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("failed to parse playlist: %v", err)
	}

	fmt.Println("Playlist uploaded successfully.")
}

func selectAndPlayChannel(addr *string) bool {
	if playlist == nil || len(playlist) == 0 {
		fmt.Println("No playlist uploaded. Please upload a playlist first.")
		return false
	}

	channelNames := make([]string, 0, len(playlist))
	for name := range playlist {
		channelNames = append(channelNames, name)
	}

	for {
		displayPlayingChannels()
		prompt := promptui.Select{
			Label: "Select Channel",
			Items: append([]string{"Back to Main Menu"}, channelNames...),
		}

		_, selectedChannel, err := prompt.Run()
		if err != nil {
			fmt.Printf("Prompt failed %v\n", err)
			return false
		}

		if selectedChannel == "Back to Main Menu" {
			return false
		}

		channelURL := playlist[selectedChannel]
		finalURL := getFinalChannelURL(channelURL)
		mu.Lock()
		playing = append(playing, selectedChannel)
		mu.Unlock()
		go func() {
			if err := runClient(*addr, finalURL); err != nil {
				log.Panicf("failed to run client: %v", err)
			}
		}()
	}
}

func getFinalChannelURL(initialURL string) string {
	resp, err := http.Get(initialURL)
	if err != nil {
		fmt.Printf("failed to fetch channel URL: %v", err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	var finalURL string
	var resolutionURLs []string

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#EXT-X-STREAM-INF:") {
			scanner.Scan()
			resolutionURLs = append(resolutionURLs, scanner.Text())
			// Skip to next resolution URL
			for scanner.Scan() && !strings.HasPrefix(scanner.Text(), "#EXT-X-STREAM-INF:") {
			}
		} else if strings.HasPrefix(line, "http") {
			finalURL = line
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("failed to parse channel URL: %v", err)
	}

	if len(resolutionURLs) > 0 {
		prompt := promptui.Select{
			Label: "Select Resolution",
			Items: resolutionURLs,
		}

		_, selectedResolution, err := prompt.Run()
		if err != nil {
			fmt.Printf("Prompt failed %v\n", err)
			return ""
		}
		finalURL = selectedResolution
	}

	if !strings.HasPrefix(finalURL, "http") {
		finalURL = initialURL[:strings.LastIndex(initialURL, "/")+1] + finalURL
	}

	return finalURL
}

func runClient(addr string, iptvAddr string) error {
	if iptvAddr == "" {
		return fmt.Errorf("iptv_addr is required")
	}
	var client *Client
	var err error
	client, err = NewQUICClient(context.Background(), addr)
	if err != nil {
		return err
	}
	return client.Run(iptvAddr)
}

func runServer(addr, certFile, keyFile string) error {
	tlsConfig, err := generateTLSConfigWithCertAndKey(certFile, keyFile)
	if err != nil {
		log.Printf("failed to generate TLS config from cert file and key, generating in memory certs: %v", err)
		tlsConfig = generateTLSConfig()
	}
	server := newServer(addr, tlsConfig)
	return server.Run()
}

func generateTLSConfigWithCertAndKey(certFile, keyFile string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"moq-00"},
	}, nil
}

// Setup a bare-bones TLS config for the server
func generateTLSConfig() *tls.Config {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		panic(err)
	}
	template := x509.Certificate{SerialNumber: big.NewInt(1)}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		panic(err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"moq-00"},
	}
}

func getChannelNameFromURL(url string) string {
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return "Unknown Channel"
}

func displayPlayingChannels() {
	mu.Lock()
	defer mu.Unlock()
	if len(playing) > 0 {
		fmt.Println("Currently playing channels:")
		for _, channel := range playing {
			fmt.Printf("- %s\n", channel)
		}
	} else {
		fmt.Println("No channels currently playing.")
	}
}
