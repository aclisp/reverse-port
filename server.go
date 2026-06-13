package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type ServerConfig struct {
	Listen                string
	StatusListen          string
	OpenTimeout           time.Duration
	MaxPendingConnections int
	MaxActiveConnections  int
	HeartbeatInterval     time.Duration
	HeartbeatTimeout      time.Duration
	Token                 string
}

const initialHeaderTimeout = 10 * time.Second

type serverState struct {
	serverListen          string
	statusListen          string
	maxPendingConnections int
	maxActiveConnections  int
	heartbeatInterval     time.Duration
	heartbeatTimeout      time.Duration
	mu                    sync.Mutex
	tunnels               map[*tunnel]struct{}
	totals                statusTotals
}

type statusTotals struct {
	AcceptedControlConnections uint64 `json:"acceptedControlConnections"`
	RejectedControlConnections uint64 `json:"rejectedControlConnections"`
	AcceptedDataConnections    uint64 `json:"acceptedDataConnections"`
	RejectedDataConnections    uint64 `json:"rejectedDataConnections"`
	RemoteConnections          uint64 `json:"remoteConnections"`
}

type statusResponse struct {
	Status       string          `json:"status"`
	ServerListen string          `json:"serverListen"`
	StatusListen string          `json:"statusListen"`
	Current      statusCurrent   `json:"current"`
	Totals       statusTotals    `json:"totals"`
	Tunnels      []tunnelSummary `json:"tunnels"`
}

type statusCurrent struct {
	ActiveTunnels      int `json:"activeTunnels"`
	PendingConnections int `json:"pendingConnections"`
	ActiveConnections  int `json:"activeConnections"`
}

type tunnelSummary struct {
	Remote             string `json:"remote"`
	Target             string `json:"target"`
	Client             string `json:"client"`
	ActiveConnections  int    `json:"activeConnections"`
	PendingConnections int    `json:"pendingConnections"`
}

type tunnel struct {
	ctx      context.Context
	cancel   context.CancelFunc
	state    *serverState
	logger   *log.Logger
	control  net.Conn
	listener net.Listener
	remote   string
	target   string
	client   string
	timeout  time.Duration
	writeMu  sync.Mutex
	mu       sync.Mutex
	pending  map[string]*pendingConn
	active   map[net.Conn]struct{}
}

type pendingConn struct {
	remote net.Conn
	timer  *time.Timer
}

func RunServer(ctx context.Context, cfg ServerConfig, logger *log.Logger) error {
	if err := validateToken(cfg.Token); err != nil {
		return err
	}
	if err := validateListenAddress(cfg.Listen, true); err != nil {
		return err
	}
	if err := validateStatusListen(cfg.StatusListen); err != nil {
		return err
	}
	if cfg.OpenTimeout <= 0 {
		return fmt.Errorf("open timeout must be positive")
	}
	if cfg.MaxPendingConnections == 0 {
		cfg.MaxPendingConnections = defaultMaxPending
	}
	if cfg.MaxActiveConnections == 0 {
		cfg.MaxActiveConnections = defaultMaxActive
	}
	if cfg.HeartbeatInterval == 0 {
		cfg.HeartbeatInterval = defaultHeartbeatInterval
	}
	if cfg.HeartbeatTimeout == 0 {
		cfg.HeartbeatTimeout = defaultHeartbeatTimeout
	}
	if cfg.MaxPendingConnections < 0 {
		return fmt.Errorf("max pending connections must be positive")
	}
	if cfg.MaxActiveConnections < 0 {
		return fmt.Errorf("max active connections must be positive")
	}
	if cfg.HeartbeatInterval < 0 {
		return fmt.Errorf("heartbeat interval must be positive")
	}
	if cfg.HeartbeatTimeout < 0 {
		return fmt.Errorf("heartbeat timeout must be positive")
	}
	if cfg.HeartbeatTimeout <= cfg.HeartbeatInterval {
		return fmt.Errorf("heartbeat timeout must be greater than heartbeat interval")
	}

	tunnelListener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return fmt.Errorf("listen %s: %w", cfg.Listen, err)
	}
	defer tunnelListener.Close()

	statusListener, err := net.Listen("tcp", cfg.StatusListen)
	if err != nil {
		return fmt.Errorf("status listen %s: %w", cfg.StatusListen, err)
	}
	defer statusListener.Close()

	state := &serverState{
		serverListen:          tunnelListener.Addr().String(),
		statusListen:          statusListener.Addr().String(),
		maxPendingConnections: cfg.MaxPendingConnections,
		maxActiveConnections:  cfg.MaxActiveConnections,
		heartbeatInterval:     cfg.HeartbeatInterval,
		heartbeatTimeout:      cfg.HeartbeatTimeout,
		tunnels:               make(map[*tunnel]struct{}),
	}
	httpServer := &http.Server{Handler: statusHandler(state)}
	go func() {
		<-ctx.Done()
		tunnelListener.Close()
		httpServer.Shutdown(context.Background())
	}()
	go func() {
		if err := httpServer.Serve(statusListener); err != nil && err != http.ErrServerClosed && !errors.Is(err, net.ErrClosed) && ctx.Err() == nil {
			logger.Printf("status server stopped: %v", err)
		}
	}()
	logger.Printf("server listening on %s", tunnelListener.Addr())
	logger.Printf("status listening on %s", statusListener.Addr())

	for {
		conn, err := tunnelListener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		enableKeepAlive(conn)
		go handleServerConn(ctx, cfg, state, logger, conn)
	}
}

func handleServerConn(ctx context.Context, cfg ServerConfig, state *serverState, logger *log.Logger, conn net.Conn) {
	line, err := readInitialHeader(conn, initialHeaderTimeout)
	if err != nil {
		state.incRejectedControl()
		logger.Printf("malformed connection from %s", conn.RemoteAddr())
		conn.Close()
		return
	}
	switch {
	case stringsHasPrefixWord(line, "CONTROL"):
		handleControlConn(ctx, cfg, state, logger, conn, line)
	case stringsHasPrefixWord(line, "DATA"):
		handleDataConn(cfg, state, logger, conn, line)
	default:
		state.incRejectedControl()
		logger.Printf("malformed connection from %s", conn.RemoteAddr())
		conn.Close()
	}
}

func readInitialHeader(conn net.Conn, timeout time.Duration) (string, error) {
	if timeout > 0 {
		if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
			return "", err
		}
	}
	line, err := readHeader(conn)
	if clearErr := conn.SetReadDeadline(time.Time{}); err == nil && clearErr != nil {
		return "", clearErr
	}
	return line, err
}

func handleControlConn(ctx context.Context, cfg ServerConfig, state *serverState, logger *log.Logger, conn net.Conn, line string) {
	h, err := parseControlHeader(line)
	if err != nil {
		state.incRejectedControl()
		logger.Printf("malformed control request from %s", conn.RemoteAddr())
		conn.Close()
		return
	}
	if !tokenEqual(h.Token, cfg.Token) {
		state.incRejectedControl()
		logger.Printf("unauthorized control request from %s", conn.RemoteAddr())
		conn.Close()
		return
	}
	remote, _ := normalizeRemoteAddress(h.Remote)
	ln, err := net.Listen("tcp", remote)
	if err != nil {
		state.incAcceptedControl()
		fmt.Fprintln(conn, "ERR remote bind failed")
		logger.Printf("remote bind failed for %s: %v", remote, err)
		conn.Close()
		return
	}
	fmt.Fprintln(conn, "OK")
	tctx, cancel := context.WithCancel(ctx)
	t := &tunnel{
		ctx:      tctx,
		cancel:   cancel,
		state:    state,
		logger:   logger,
		control:  conn,
		listener: ln,
		remote:   ln.Addr().String(),
		target:   h.Target,
		client:   conn.RemoteAddr().String(),
		timeout:  cfg.OpenTimeout,
		pending:  make(map[string]*pendingConn),
		active:   make(map[net.Conn]struct{}),
	}
	state.addTunnel(t)
	state.incAcceptedControl()
	logger.Printf("remote listener ready on %s for %s", t.remote, t.client)
	t.run()
}

func (t *tunnel) run() {
	defer t.close()
	go t.monitorControl()
	for {
		remoteConn, err := t.listener.Accept()
		if err != nil {
			return
		}
		enableKeepAlive(remoteConn)
		t.state.incRemote()
		if !t.canAcceptRemote() {
			t.logger.Printf("remote connection rejected on %s: tunnel capacity reached", t.remote)
			remoteConn.Close()
			continue
		}
		id, err := randomID()
		if err != nil {
			remoteConn.Close()
			continue
		}
		pc := &pendingConn{remote: remoteConn}
		pc.timer = time.AfterFunc(t.timeout, func() {
			t.mu.Lock()
			if pending, ok := t.pending[id]; ok {
				delete(t.pending, id)
				pending.remote.Close()
			}
			t.mu.Unlock()
		})
		t.mu.Lock()
		t.pending[id] = pc
		t.mu.Unlock()
		t.writeMu.Lock()
		err = writeOpen(t.control, id)
		t.writeMu.Unlock()
		if err != nil {
			remoteConn.Close()
			return
		}
	}
}

func (t *tunnel) canAcceptRemote() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.pending) >= t.state.maxPendingConnections {
		return false
	}
	return t.activeConnectionCountLocked() < t.state.maxActiveConnections
}

func (t *tunnel) monitorControl() {
	ticker := time.NewTicker(t.state.heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-t.ctx.Done():
			return
		case <-ticker.C:
		}
		t.writeMu.Lock()
		err := writePing(t.control)
		t.writeMu.Unlock()
		if err != nil {
			t.closeForControlFailure()
			return
		}
		if err := t.control.SetReadDeadline(time.Now().Add(t.state.heartbeatTimeout)); err != nil {
			t.closeForControlFailure()
			return
		}
		line, err := readHeader(t.control)
		_ = t.control.SetReadDeadline(time.Time{})
		if err != nil {
			t.closeForControlFailure()
			return
		}
		if line != "PONG" {
			t.logger.Printf("control connection protocol error from %s", t.client)
			t.closeForControlFailure()
			return
		}
	}
}

func (t *tunnel) closeForControlFailure() {
	t.cancel()
	t.control.Close()
	t.listener.Close()
}

func handleDataConn(cfg ServerConfig, state *serverState, logger *log.Logger, conn net.Conn, line string) {
	h, err := parseDataHeader(line)
	if err != nil || !tokenEqual(h.Token, cfg.Token) {
		state.incRejectedData()
		logger.Printf("rejected data connection from %s", conn.RemoteAddr())
		fmt.Fprintln(conn, "ERR")
		conn.Close()
		return
	}
	t, pc := state.takePending(h.ID)
	if t == nil || pc == nil {
		state.incRejectedData()
		logger.Printf("rejected data connection from %s", conn.RemoteAddr())
		fmt.Fprintln(conn, "ERR")
		conn.Close()
		return
	}
	if !t.canActivate() {
		state.incRejectedData()
		logger.Printf("rejected data connection from %s: tunnel active capacity reached", conn.RemoteAddr())
		conn.Close()
		pc.timer.Stop()
		pc.remote.Close()
		return
	}
	state.incAcceptedData()
	pc.timer.Stop()
	t.addActive(conn)
	t.addActive(pc.remote)
	defer t.removeActive(conn)
	defer t.removeActive(pc.remote)
	pipeBidirectional(t.ctx, pc.remote, conn)
}

func (t *tunnel) close() {
	t.cancel()
	t.listener.Close()
	t.control.Close()
	t.mu.Lock()
	for id, pc := range t.pending {
		delete(t.pending, id)
		pc.timer.Stop()
		pc.remote.Close()
	}
	for conn := range t.active {
		conn.Close()
	}
	t.mu.Unlock()
	t.state.removeTunnel(t)
	t.logger.Printf("remote listener closed on %s for %s", t.remote, t.client)
}

func (t *tunnel) addActive(conn net.Conn) {
	t.mu.Lock()
	t.active[conn] = struct{}{}
	t.mu.Unlock()
}

func (t *tunnel) canActivate() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.activeConnectionCountLocked() < t.state.maxActiveConnections
}

func (t *tunnel) removeActive(conn net.Conn) {
	t.mu.Lock()
	delete(t.active, conn)
	t.mu.Unlock()
}

func (t *tunnel) activeConnectionCountLocked() int {
	return len(t.active) / 2
}

func (s *serverState) takePending(id string) (*tunnel, *pendingConn) {
	s.mu.Lock()
	tunnels := make([]*tunnel, 0, len(s.tunnels))
	for t := range s.tunnels {
		tunnels = append(tunnels, t)
	}
	s.mu.Unlock()
	for _, t := range tunnels {
		t.mu.Lock()
		pc, ok := t.pending[id]
		if ok {
			delete(t.pending, id)
		}
		t.mu.Unlock()
		if ok {
			return t, pc
		}
	}
	return nil, nil
}

func (s *serverState) addTunnel(t *tunnel) {
	s.mu.Lock()
	s.tunnels[t] = struct{}{}
	s.mu.Unlock()
}

func (s *serverState) removeTunnel(t *tunnel) {
	s.mu.Lock()
	delete(s.tunnels, t)
	s.mu.Unlock()
}

func (s *serverState) incAcceptedControl() { atomic.AddUint64(&s.totals.AcceptedControlConnections, 1) }
func (s *serverState) incRejectedControl() { atomic.AddUint64(&s.totals.RejectedControlConnections, 1) }
func (s *serverState) incAcceptedData()    { atomic.AddUint64(&s.totals.AcceptedDataConnections, 1) }
func (s *serverState) incRejectedData()    { atomic.AddUint64(&s.totals.RejectedDataConnections, 1) }
func (s *serverState) incRemote()          { atomic.AddUint64(&s.totals.RemoteConnections, 1) }

func (s *serverState) snapshot() statusResponse {
	s.mu.Lock()
	tunnels := make([]*tunnel, 0, len(s.tunnels))
	for t := range s.tunnels {
		tunnels = append(tunnels, t)
	}
	s.mu.Unlock()
	resp := statusResponse{
		Status:       "ok",
		ServerListen: s.serverListen,
		StatusListen: s.statusListen,
		Totals: statusTotals{
			AcceptedControlConnections: atomic.LoadUint64(&s.totals.AcceptedControlConnections),
			RejectedControlConnections: atomic.LoadUint64(&s.totals.RejectedControlConnections),
			AcceptedDataConnections:    atomic.LoadUint64(&s.totals.AcceptedDataConnections),
			RejectedDataConnections:    atomic.LoadUint64(&s.totals.RejectedDataConnections),
			RemoteConnections:          atomic.LoadUint64(&s.totals.RemoteConnections),
		},
		Tunnels: make([]tunnelSummary, 0, len(tunnels)),
	}
	for _, t := range tunnels {
		t.mu.Lock()
		pending := len(t.pending)
		active := len(t.active) / 2
		summary := tunnelSummary{
			Remote:             t.remote,
			Target:             t.target,
			Client:             t.client,
			ActiveConnections:  active,
			PendingConnections: pending,
		}
		t.mu.Unlock()
		resp.Current.ActiveTunnels++
		resp.Current.PendingConnections += pending
		resp.Current.ActiveConnections += active
		resp.Tunnels = append(resp.Tunnels, summary)
	}
	return resp
}

func statusHandler(state *serverState) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(state.snapshot())
	})
	return mux
}

func randomID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func enableKeepAlive(conn net.Conn) {
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetKeepAlive(true)
	}
}

func stringsHasPrefixWord(s, word string) bool {
	return s == word || len(s) > len(word) && s[:len(word)] == word && s[len(word)] == ' '
}
