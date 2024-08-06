package main

import (
	"context"
	"crypto/tls"
	"log"
	"net/http"
	"time"

	"github.com/mengelbart/moqtransport"
	"github.com/mengelbart/moqtransport/quicmoq"
	"github.com/mengelbart/moqtransport/webtransportmoq"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/webtransport-go"
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
	wt := webtransport.Server{
		H3: http3.Server{
			Addr:      s.addr,
			TLSConfig: s.tlsConfig,
		},
	}
	http.HandleFunc("/moq", func(w http.ResponseWriter, r *http.Request) {
		session, err := wt.Upgrade(w, r)
		if err != nil {
			log.Printf("failed to upgrade webtransport request: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		moqSession := &moqtransport.Session{
			Conn:                webtransportmoq.New(session),
			EnableDatagrams:     false,
			LocalRole:           moqtransport.RolePubSub,
			RemoteRole:          moqtransport.RolePubSub,
			AnnouncementHandler: nil,
			SubscriptionHandler: s.sessionManager,
		}
		if err := moqSession.RunServer(r.Context()); err != nil {
			log.Printf("failed to run server session handshake: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	})
	for {
		conn, err := listener.Accept(ctx)
		if err != nil {
			return err
		}
		switch conn.ConnectionState().TLS.NegotiatedProtocol {
		case "h3":
			go wt.ServeQUICConn(conn)
		case "moq-00":
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
		default:
			log.Printf("unknown protocol: %v", conn.ConnectionState().TLS.NegotiatedProtocol)
			conn.CloseWithError(quic.ApplicationErrorCode(0x02), "unknown protocol")
		}
	}
}
