package main

import (
	"context"
	"crypto/tls"
	"log"
	"time"

	"github.com/mengelbart/moqtransport"
	"github.com/mengelbart/moqtransport/quicmoq"
	"github.com/quic-go/quic-go"
)

type server struct {
	addr      string
	tlsConfig *tls.Config

	sessionManager *sessionManager
}

func newServer(addr string, tlsConfig *tls.Config) *server {
	return &server{
		addr:           addr,
		tlsConfig:      tlsConfig,
		sessionManager: newSessionManager(),
	}
}

func (s *server) Run() error {
	ctx := context.Background()

	listener, err := quic.ListenAddr(s.addr, s.tlsConfig, &quic.Config{
		EnableDatagrams: true,
		MaxIdleTimeout:  time.Hour,
	})
	if err != nil {
		return err
	}
	for {
		conn, err := listener.Accept(ctx)
		if err != nil {
			return err
		}
		if conn.ConnectionState().TLS.NegotiatedProtocol == "moq-00" {
			p := &moqtransport.Session{
				Conn:                quicmoq.New(conn),
				EnableDatagrams:     true,
				LocalRole:           moqtransport.RolePubSub,
				AnnouncementHandler: nil,
				SubscriptionHandler: s.sessionManager,
			}
			if err := p.RunServer(ctx); err != nil {
				p.Close()
				log.Printf("err opening moqtransport server session: %v", err)
				continue
			}

			var matchedChannel *channel
			for _, channel := range s.sessionManager.channels {
				if channel.session == p {
					matchedChannel = channel
					break
				}
			}

			if matchedChannel != nil {
				go matchedChannel.serve()
			}
		} else {
			log.Printf("unknown protocol: %v", conn.ConnectionState().TLS.NegotiatedProtocol)
			conn.CloseWithError(quic.ApplicationErrorCode(0x02), "unknown protocol")
		}
	}
}
