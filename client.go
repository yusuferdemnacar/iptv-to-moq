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

func (c *Client) play(channelID string) error {

	videoTrack, err := c.session.Subscribe(context.Background(), 2, 0, fmt.Sprintf("iptv-moq/%v", channelID), "video", "")
	if err != nil {
		fmt.Printf("failed to subscribe: %v", err)
		return err
	}
	audioTrack, err := c.session.Subscribe(context.Background(), 3, 0, fmt.Sprintf("iptv-moq/%v", channelID), "audio", "")
	if err != nil {
		fmt.Printf("failed to subscribe: %v", err)
		return err
	}

	cmd := exec.Command("/mnt/c/ffmpeg/bin/ffplay.exe", "-") // for WSL2 that can't run it's own ffplay
	// cmd = exec.Command("ffplay", "-") // for all other cases where ffplay runs properly
	stdin, err := cmd.StdinPipe()
	// cmd.Stdout = os.Stdout
	// cmd.Stderr = os.Stderr
	if err != nil {
		fmt.Printf("failed to get stdin pipe: %v", err)
		return err
	}

	// Set the buffer size to a larger value (e.g., 65536 bytes)
	// Remove this block for windows as windows adjusts the buffer size automatically
	newBufferSize := 65536 * 4

	file, ok := stdin.(*os.File)
	if !ok {
		fmt.Printf("stdin is not of type *os.File")
		return fmt.Errorf("stdin is not of type *os.File")
	}

	fd := file.Fd()
	if _, _, errno := syscall.Syscall(syscall.SYS_FCNTL, fd, syscall.F_SETPIPE_SZ, uintptr(newBufferSize)); errno != 0 {
		fmt.Printf("failed to set pipe buffer size: %v", errno)
	} else {
		log.Printf("set pipe buffer size to %d bytes", newBufferSize)
	}

	if err := cmd.Start(); err != nil {
		fmt.Printf("failed to start command: %v", err)
		return err
	}

	defer stdin.Close()

	// TODO read video and audio tracks asynchronusly
	//     - need thread safe reads on the client side

	go func() {

		for i := 0; i < 2; i++ {
			ov, err := videoTrack.ReadObject(context.Background())
			if err != nil {
				fmt.Printf("failed to read object: %v", err)
				return
			}
			_, err = stdin.Write(ov.Payload)
			if err != nil {
				fmt.Printf("failed to write object: %v", err)
				return
			}
		}

		for {
			ov, err := videoTrack.ReadObject(context.Background())
			if err != nil {
				fmt.Printf("failed to read object: %v", err)
				return
			}
			_, err = stdin.Write(ov.Payload)
			if err != nil {
				fmt.Printf("failed to write object: %v", err)
				return
			}

			oa, err := audioTrack.ReadObject(context.Background())
			if err != nil {
				fmt.Printf("failed to read object: %v", err)
				return
			}
			_, err = stdin.Write(oa.Payload)
			if err != nil {
				fmt.Printf("failed to write object: %v", err)
				return
			}

		}
	}()

	err = cmd.Wait()
	if err != nil {
		log.Printf("ffplay exited with error: %v", err)
	} else {
		log.Printf("ffplay exited successfully")
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
