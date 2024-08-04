package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/mengelbart/moqtransport"
	"github.com/mengelbart/moqtransport/quicmoq"
	"github.com/mengelbart/moqtransport/webtransportmoq"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/webtransport-go"
)

type Client struct {
	session *moqtransport.Session
}

func NewQUICClient(ctx context.Context, addr string) (*Client, error) {
	conn, err := quic.DialAddr(ctx, addr, &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"moq-00"},
	}, &quic.Config{
		EnableDatagrams: true,
		MaxIdleTimeout:  time.Hour,
	})
	if err != nil {
		return nil, err
	}
	return NewClient(quicmoq.New(conn))
}

func NewWebTransportClient(ctx context.Context, addr string) (*Client, error) {
	dialer := webtransport.Dialer{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
		QUICConfig: &quic.Config{
			EnableDatagrams: true,
			MaxIdleTimeout:  time.Hour,
		},
	}
	_, session, err := dialer.Dial(ctx, addr, nil)
	if err != nil {
		return nil, err
	}
	return NewClient(webtransportmoq.New(session))
}

func NewClient(conn moqtransport.Connection) (*Client, error) {
	log.SetOutput(io.Discard)
	moqSession := &moqtransport.Session{
		Conn:                conn,
		EnableDatagrams:     true,
		LocalRole:           moqtransport.RoleSubscriber,
		RemoteRole:          moqtransport.RolePubSub,
		AnnouncementHandler: nil,
		SubscriptionHandler: nil,
	}
	if err := moqSession.RunClient(); err != nil {
		return nil, err
	}
	return &Client{
		session: moqSession,
	}, nil
}

func (c *Client) getChannel(channelID string) error {

	t, err := c.session.Subscribe(context.Background(), 2, 0, fmt.Sprintf("iptv-moq/%v", channelID), "video", "")
	if err != nil {
		log.Fatalf("failed to subscribe: %v", err)
		return err
	}

	cmd := exec.Command("ffplay", "-")
	stdin, err := cmd.StdinPipe()
	// cmd.Stdout = os.Stdout
	// cmd.Stderr = os.Stderr
	if err != nil {
		log.Fatalf("failed to get stdin pipe: %v", err)
		return err
	}

	// Set the buffer size to a larger value (e.g., 65536 bytes)
	newBufferSize := 65536 * 4

	file, ok := stdin.(*os.File)
	if !ok {
		log.Fatalf("stdin is not of type *os.File")
		return fmt.Errorf("stdin is not of type *os.File")
	}

	fd := file.Fd()
	if _, _, errno := syscall.Syscall(syscall.SYS_FCNTL, fd, syscall.F_SETPIPE_SZ, uintptr(newBufferSize)); errno != 0 {
		log.Fatalf("failed to set pipe buffer size: %v", errno)
	} else {
		log.Printf("set pipe buffer size to %d bytes", newBufferSize)
	}

	if err := cmd.Start(); err != nil {
		log.Fatalf("failed to start command: %v", err)
		return err
	}

	defer stdin.Close()

	for {
		o, err := t.ReadObject(context.Background())
		if err != nil {
			log.Fatalf("failed to read object: %v", err)
			return err
		}

		// fmt.Printf("Read object: %d bytes\n", len(o.Payload))

		_, err = stdin.Write(o.Payload)
		if err != nil {
			log.Fatalf("failed to write object: %v", err)
			return err
		}

		// fmt.Printf("Wrote object: %d bytes\n", len(o.Payload))
	}
}

func (c *Client) Run(iptvAddr string) error {
	err := c.getChannel(iptvAddr)
	if err != nil {
		return err
	}
	return nil
}
