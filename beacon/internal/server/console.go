package server

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"gamepanel/beacon/internal/runtime"
	"gamepanel/beacon/internal/system"
)

// ConsoleThrottle tracks the rate of console output and fires a strike
// callback exactly once when the rate limit is exceeded. When output falls
// back under the limit, the lock is released so the strike can fire again
// on the next violation.
type ConsoleThrottle struct {
	limit  *system.Rate
	lock   *system.Locker
	strike func()
}

func newConsoleThrottle(lines uint64, period time.Duration) *ConsoleThrottle {
	return &ConsoleThrottle{
		limit: system.NewRate(lines, period),
		lock:  system.NewLocker(),
	}
}

// Allow checks if the console is allowed to process more output data, or if too
// much has already been sent over the line. If there is too much output the
// strike callback function is triggered, but only if it has not already been
// triggered at this point in the process.
//
// If output is allowed, the lock on the throttler is released and the next time
// it is triggered the strike function will be re-executed.
func (ct *ConsoleThrottle) Allow() bool {
	if !ct.limit.Try() {
		if err := ct.lock.Acquire(); err == nil {
			if ct.strike != nil {
				ct.strike()
			}
		}
		return false
	}
	ct.lock.Release()
	return true
}

// Reset resets the console throttler internal rate limiter and overage counter.
func (ct *ConsoleThrottle) Reset() {
	ct.limit.Reset()
}

const (
	consoleReplayEntries  = 128
	consoleReplayBytes    = 256 * 1024
	consoleSubscriberSize = 64
)

// consoleManager owns at most one live runtime console session per server. A
// producer never shares state or channels with another server, and subscriber
// delivery is bounded so a slow websocket cannot block runtime output.
type consoleManager struct {
	ctx       context.Context
	runtime   runtime.Runtime
	mu        sync.Mutex
	producers map[string]*consoleProducer
}

type consoleProducer struct {
	serverID string
	session  runtime.ConsoleSession
	cancel   context.CancelFunc
	done     chan struct{}
	owner    *consoleManager

	writeMu sync.Mutex
	mu      sync.Mutex
	replay  [][]byte
	bytes   int
	subs    map[chan []byte]struct{}
	closed  bool
}

func newConsoleManager(ctx context.Context, rt runtime.Runtime) *consoleManager {
	return &consoleManager{ctx: ctx, runtime: rt, producers: make(map[string]*consoleProducer)}
}

func (m *consoleManager) Ensure(serverID string) error {
	if m == nil || m.runtime == nil {
		return errRuntimeUnavailable
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.producers[serverID]; ok {
		return nil
	}
	ctx, cancel := context.WithCancel(m.ctx)
	session, err := m.runtime.AttachConsole(ctx, serverID)
	if err != nil {
		cancel()
		return err
	}
	producer := &consoleProducer{
		serverID: serverID,
		session:  session,
		cancel:   cancel,
		done:     make(chan struct{}),
		owner:    m,
		subs:     make(map[chan []byte]struct{}),
	}
	m.producers[serverID] = producer
	go producer.run()
	return nil
}

func (p *consoleProducer) run() {
	defer close(p.done)
	buffer := make([]byte, 32*1024)
	for {
		n, err := p.session.Read(buffer)
		if n > 0 {
			p.publish(buffer[:n])
		}
		if err != nil {
			p.owner.detach(p)
			return
		}
	}
}

func (p *consoleProducer) publish(data []byte) {
	message := append([]byte(nil), data...)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.replay = append(p.replay, message)
	p.bytes += len(message)
	for len(p.replay) > consoleReplayEntries || p.bytes > consoleReplayBytes {
		p.bytes -= len(p.replay[0])
		p.replay[0] = nil
		p.replay = p.replay[1:]
	}
	for subscriber := range p.subs {
		copyForSubscriber := append([]byte(nil), message...)
		select {
		case subscriber <- copyForSubscriber:
		default:
			select {
			case <-subscriber:
			default:
			}
			select {
			case subscriber <- copyForSubscriber:
			default:
			}
		}
	}
}

func (m *consoleManager) Subscribe(serverID string) (<-chan []byte, func(), error) {
	if m == nil {
		return nil, nil, errRuntimeUnavailable
	}
	m.mu.Lock()
	producer := m.producers[serverID]
	if producer == nil {
		m.mu.Unlock()
		return nil, nil, errors.New("server console is not running")
	}
	producer.mu.Lock()
	m.mu.Unlock()
	if producer.closed {
		producer.mu.Unlock()
		return nil, nil, errors.New("server console is not running")
	}
	channel := make(chan []byte, consoleReplayEntries+consoleSubscriberSize)
	for _, entry := range producer.replay {
		channel <- append([]byte(nil), entry...)
	}
	producer.subs[channel] = struct{}{}
	producer.mu.Unlock()

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			producer.mu.Lock()
			if _, ok := producer.subs[channel]; ok {
				delete(producer.subs, channel)
				close(channel)
			}
			producer.mu.Unlock()
		})
	}
	return channel, unsubscribe, nil
}

func (m *consoleManager) Write(serverID, command string) error {
	if err := m.Ensure(serverID); err != nil {
		return err
	}
	m.mu.Lock()
	producer := m.producers[serverID]
	m.mu.Unlock()
	if producer == nil {
		return errors.New("server console is not running")
	}
	producer.writeMu.Lock()
	defer producer.writeMu.Unlock()
	if command == "" {
		return nil
	}
	if command[len(command)-1] != '\n' {
		command += "\n"
	}
	_, err := io.WriteString(producer.session, command)
	return err
}

func (m *consoleManager) detach(producer *consoleProducer) {
	m.mu.Lock()
	if m.producers[producer.serverID] == producer {
		delete(m.producers, producer.serverID)
	}
	m.mu.Unlock()
	producer.closeSubscribers()
}

func (p *consoleProducer) closeSubscribers() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.closed = true
	for subscriber := range p.subs {
		close(subscriber)
		delete(p.subs, subscriber)
	}
}

func (m *consoleManager) Stop(serverID string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	producer := m.producers[serverID]
	if producer != nil {
		delete(m.producers, serverID)
	}
	m.mu.Unlock()
	if producer == nil {
		return
	}
	producer.closeSubscribers()
	producer.cancel()
	_ = producer.session.Close()
	<-producer.done
}

func (m *consoleManager) Close() {
	if m == nil {
		return
	}
	m.mu.Lock()
	ids := make([]string, 0, len(m.producers))
	for id := range m.producers {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		m.Stop(id)
	}
}

func (m *consoleManager) producerCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.producers)
}
