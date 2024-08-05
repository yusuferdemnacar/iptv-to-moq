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
	"github.com/quic-go/quic-go"
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

func (c *Client) play(channelID string) error {

	videoTrack, err := c.session.Subscribe(context.Background(), 2, 0, fmt.Sprintf("iptv-moq/%v", channelID), "video", "")
	if err != nil {
		log.Fatalf("failed to subscribe: %v", err)
		return err
	}
	audioTrack, err := c.session.Subscribe(context.Background(), 3, 0, fmt.Sprintf("iptv-moq/%v", channelID), "audio", "")
	if err != nil {
		log.Fatalf("failed to subscribe: %v", err)
		return err
	}

	cmd := exec.Command("/mnt/c/ffmpeg/bin/ffplay.exe", "-") // for WSL2 that can't run it's own ffplay
	// cmd = exec.Command("ffplay", "-") // for Linux
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

	// TODO read video and audio tracks asynchronusly

	go func() {

		for i := 0; i < 2; i++ {
			ov, err := videoTrack.ReadObject(context.Background())
			if err != nil {
				log.Fatalf("failed to read object: %v", err)
				return
			}
			_, err = stdin.Write(ov.Payload)
			if err != nil {
				log.Fatalf("failed to write object: %v", err)
				return
			}
		}

		for {
			ov, err := videoTrack.ReadObject(context.Background())
			if err != nil {
				log.Fatalf("failed to read object: %v", err)
				return
			}
			_, err = stdin.Write(ov.Payload)
			if err != nil {
				log.Fatalf("failed to write object: %v", err)
				return
			}

			oa, err := audioTrack.ReadObject(context.Background())
			if err != nil {
				log.Fatalf("failed to read object: %v", err)
				return
			}
			_, err = stdin.Write(oa.Payload)
			if err != nil {
				log.Fatalf("failed to write object: %v", err)
				return
			}

		}
	}()

	err = cmd.Wait()
	if err != nil {
		log.Printf("ffplay exited with error: %v", err)
	} else {
		log.Println("ffplay exited successfully")
	}

	videoTrack.Unsubscribe()
	audioTrack.Unsubscribe()

	time.Sleep(1 * time.Second)

	return nil
}

func (c *Client) Run(iptvAddr string) error {
	err := c.play(iptvAddr)
	if err != nil {
		return err
	}
	return nil
}
