package main

import (
	"fmt"
	"strings"
	"sync"

	"github.com/mengelbart/moqtransport"
)

type errorCode uint64

const (
	errorCodeInvalidNamespace errorCode = iota + 1
	errorCodeInternal
	errorCodeUnknownRoom
	errorCodeDuplicateUsername
	errorCodeUnknownParticipant
)

type sessionManager struct {
	channels     map[channelID]*channel
	channelsLock sync.Mutex

	sessions     []*moqtransport.Session
	sessionsLock sync.Mutex
}

func newSessionManager() *sessionManager {
	return &sessionManager{
		channels:     map[channelID]*channel{},
		sessions:     []*moqtransport.Session{},
		sessionsLock: sync.Mutex{},
	}
}

func (m *sessionManager) handle(s *moqtransport.Session) {
	m.sessionsLock.Lock()
	defer m.sessionsLock.Unlock()
	m.sessions = append(m.sessions, s)
	fmt.Println("session added")
	fmt.Println("sessions:", len(m.sessions))
}

func (m *sessionManager) HandleSubscription(s *moqtransport.Session, sub *moqtransport.Subscription, srw moqtransport.SubscriptionResponseWriter) {

	var parts []string
	if !strings.Contains(sub.Namespace, "/") {
		srw.Reject(uint64(errorCodeInvalidNamespace), "namespace MUST contain at least one '/'")
		return
	}

	index := strings.Index(sub.Namespace, "/")
	parts = append(parts, sub.Namespace[:index], sub.Namespace[index+1:])
	iptv, id := parts[0], channelID(parts[1])
	if iptv != "iptv-moq" {
		srw.Reject(uint64(errorCodeInvalidNamespace), "first part of namespace MUST equal 'iptv-moq'")
		return
	}

	m.channelsLock.Lock()
	defer m.channelsLock.Unlock()
	channel, ok := m.channels[id]
	if !ok {
		channel = newChannel(id)
		m.channels[id] = channel
	}

	go channel.subscribe(s, srw)
}
