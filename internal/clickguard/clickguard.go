// Package clickguard provides a small, dependency-free click captcha and
// in-memory request limiter for the TTS synthesis endpoint.
package clickguard

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"
)

const (
	challengeTTL = 2 * time.Minute
	ticketTTL    = 2 * time.Minute
	tolerance    = 35
)

var (
	iconIDs = []string{"sun", "heart", "star", "cloud", "leaf", "droplet", "flower", "diamond"}
	errBad  = errors.New("invalid captcha data")
)

// Config controls request verification and the captcha challenge.
type Config struct {
	Enabled bool
	Limit   int
	Window  time.Duration
}

// DefaultConfig returns the application defaults.
func DefaultConfig() Config {
	return Config{
		Enabled: true,
		Limit:   10,
		Window:  10 * time.Minute,
	}
}

// Challenge is safe to send to the browser. Coordinates remain encrypted in
// Code and are never exposed as plaintext.
type Challenge struct {
	Type      string   `json:"type"`
	Image     string   `json:"image"`
	Targets   []string `json:"targets"`
	Code      string   `json:"clickCode"`
	ExpiresIn int      `json:"expiresIn"`
}

// VerifyResult is returned after a successful click verification.
type VerifyResult struct {
	Ticket    string `json:"ticket"`
	ExpiresIn int    `json:"expiresIn"`
}

type point struct {
	Icon string `json:"icon"`
	X    int    `json:"x"`
	Y    int    `json:"y"`
}

type challengeData struct {
	Points    []point `json:"points"`
	ExpiresAt int64   `json:"expiresAt"`
	ID        string  `json:"id"`
}

type ticketData struct {
	Client    string `json:"client"`
	ExpiresAt int64  `json:"expiresAt"`
	ID        string `json:"id"`
}

// Manager tracks rolling request windows and issues/validates captcha data.
type Manager struct {
	cfg  Config
	aead cipher.AEAD
	now  func() time.Time

	mu             sync.Mutex
	buckets        map[string][]time.Time
	usedChallenges map[string]time.Time
	usedTickets    map[string]time.Time
	requests       uint64
}

// New creates a Manager with a process-local encryption key.
func New(cfg Config) (*Manager, error) {
	if cfg.Limit <= 0 {
		cfg.Limit = DefaultConfig().Limit
	}
	if cfg.Window <= 0 {
		cfg.Window = DefaultConfig().Window
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate captcha key: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create captcha cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create captcha GCM: %w", err)
	}
	return &Manager{
		cfg:            cfg,
		aead:           aead,
		now:            time.Now,
		buckets:        make(map[string][]time.Time),
		usedChallenges: make(map[string]time.Time),
		usedTickets:    make(map[string]time.Time),
	}, nil
}

// Enabled reports whether request verification is enabled.
func (m *Manager) Enabled() bool {
	return m != nil && m.cfg.Enabled
}

// Limit returns the configured number of allowed requests per window.
func (m *Manager) Limit() int {
	if m == nil {
		return 0
	}
	return m.cfg.Limit
}

// Window returns the configured rolling window.
func (m *Manager) Window() time.Duration {
	if m == nil {
		return 0
	}
	return m.cfg.Window
}

// Allow records a request when both the IP and cookie identity are below the
// limit. If either identity has reached the limit, the request is rejected.
func (m *Manager) Allow(ip, client string) bool {
	return m.allowAt(ip, client, m.now())
}

func (m *Manager) allowAt(ip, client string, now time.Time) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.requests++
	if m.requests%256 == 0 {
		m.cleanupLocked(now)
	}
	keys := identityKeys(ip, client)
	if len(keys) == 0 {
		return false
	}
	for _, key := range keys {
		bucket := pruneBucket(m.buckets[key], now.Add(-m.cfg.Window))
		m.buckets[key] = bucket
		if len(bucket) >= m.cfg.Limit {
			return false
		}
	}
	for _, key := range keys {
		m.buckets[key] = append(m.buckets[key], now)
	}
	return true
}

// Reset clears both identities after a valid verification ticket is consumed.
func (m *Manager) Reset(ip, client string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, key := range identityKeys(ip, client) {
		delete(m.buckets, key)
	}
}

// NewChallenge creates an icon click challenge.
func (m *Manager) NewChallenge() (*Challenge, error) {
	return m.newChallengeAt(m.now())
}

func (m *Manager) newChallengeAt(now time.Time) (*Challenge, error) {
	selected := make([]string, 3)
	used := make(map[int]bool, len(selected))
	for i := range selected {
		for {
			index, err := secureInt(len(iconIDs))
			if err != nil {
				return nil, err
			}
			if !used[index] {
				used[index] = true
				selected[i] = iconIDs[index]
				break
			}
		}
	}
	return m.buildChallenge(placeIcons(selected), now)
}

func (m *Manager) buildChallenge(points []point, now time.Time) (*Challenge, error) {
	imageURL, err := renderChallenge(points)
	if err != nil {
		return nil, err
	}
	challengeID, err := randomToken(18)
	if err != nil {
		return nil, err
	}
	data := challengeData{
		Points:    points,
		ExpiresAt: now.Add(challengeTTL).Unix(),
		ID:        challengeID,
	}
	code, err := m.seal(data)
	if err != nil {
		return nil, err
	}
	targets := make([]string, len(points))
	for i, item := range points {
		targets[i] = renderIconPreview(item.Icon)
	}
	return &Challenge{
		Type:      "click",
		Image:     imageURL,
		Targets:   targets,
		Code:      code,
		ExpiresIn: int(challengeTTL / time.Second),
	}, nil
}

// Verify checks three click coordinates in target-icon order and returns a
// short-lived, single-use ticket on success.
func (m *Manager) Verify(code, clickXs, clickYs, client string) (*VerifyResult, error) {
	xs, err := parsePoints(clickXs)
	if err != nil {
		return nil, err
	}
	ys, err := parsePoints(clickYs)
	if err != nil {
		return nil, err
	}
	return m.verifyAt(code, xs, ys, client, m.now())
}

func (m *Manager) verifyAt(code string, xs, ys []int, client string, now time.Time) (*VerifyResult, error) {
	if len(xs) != 3 || len(ys) != 3 {
		return nil, errBad
	}
	var challenge challengeData
	if err := m.open(code, &challenge); err != nil {
		return nil, errBad
	}
	if challenge.ID == "" || now.Unix() >= challenge.ExpiresAt || len(challenge.Points) != 3 {
		return nil, errBad
	}
	for i, expected := range challenge.Points {
		dx := xs[i] - expected.X
		dy := ys[i] - expected.Y
		if dx*dx+dy*dy > tolerance*tolerance {
			return nil, errBad
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	now = now.Truncate(time.Second)
	if expiry, exists := m.usedChallenges[challenge.ID]; exists && expiry.After(now) {
		return nil, errBad
	}
	m.usedChallenges[challenge.ID] = time.Unix(challenge.ExpiresAt, 0)
	ticketID, err := randomToken(18)
	if err != nil {
		return nil, err
	}
	ticket := ticketData{
		Client:    clientHash(client),
		ExpiresAt: now.Add(ticketTTL).Unix(),
		ID:        ticketID,
	}
	value, err := m.seal(ticket)
	if err != nil {
		return nil, err
	}
	return &VerifyResult{Ticket: value, ExpiresIn: int(ticketTTL / time.Second)}, nil
}

// ConsumeTicket validates and consumes a verification ticket for one request.
func (m *Manager) ConsumeTicket(value, client string) bool {
	var ticket ticketData
	if err := m.open(value, &ticket); err != nil {
		return false
	}
	now := m.now().Truncate(time.Second)
	if ticket.ID == "" || ticket.Client != clientHash(client) || now.Unix() >= ticket.ExpiresAt {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if expiry, exists := m.usedTickets[ticket.ID]; exists && expiry.After(now) {
		return false
	}
	m.usedTickets[ticket.ID] = time.Unix(ticket.ExpiresAt, 0)
	return true
}

func (m *Manager) seal(value any) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, m.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := m.aead.Seal(nonce, nonce, payload, nil)
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (m *Manager) open(value string, target any) error {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(raw) < m.aead.NonceSize() {
		return errBad
	}
	nonce := raw[:m.aead.NonceSize()]
	plain, err := m.aead.Open(nil, nonce, raw[m.aead.NonceSize():], nil)
	if err != nil {
		return errBad
	}
	if err := json.Unmarshal(plain, target); err != nil {
		return errBad
	}
	return nil
}

func (m *Manager) cleanupLocked(now time.Time) {
	cutoff := now.Add(-m.cfg.Window)
	for key, bucket := range m.buckets {
		bucket = pruneBucket(bucket, cutoff)
		if len(bucket) == 0 {
			delete(m.buckets, key)
		} else {
			m.buckets[key] = bucket
		}
	}
	for key, expiry := range m.usedChallenges {
		if !expiry.After(now) {
			delete(m.usedChallenges, key)
		}
	}
	for key, expiry := range m.usedTickets {
		if !expiry.After(now) {
			delete(m.usedTickets, key)
		}
	}
}

func identityKeys(ip, client string) []string {
	keys := make([]string, 0, 2)
	if ip = strings.TrimSpace(ip); ip != "" {
		keys = append(keys, "ip:"+ip)
	}
	if client = strings.TrimSpace(client); client != "" {
		keys = append(keys, "cookie:"+client)
	}
	return keys
}

func pruneBucket(bucket []time.Time, cutoff time.Time) []time.Time {
	first := 0
	for first < len(bucket) && bucket[first].Before(cutoff) {
		first++
	}
	if first == len(bucket) {
		return bucket[:0]
	}
	return bucket[first:]
}

func parsePoints(value string) ([]int, error) {
	parts := strings.Split(strings.TrimSpace(value), ",")
	if len(parts) != 3 {
		return nil, errBad
	}
	result := make([]int, len(parts))
	for i, part := range parts {
		point, err := parseSmallInt(part)
		if err != nil {
			return nil, errBad
		}
		result[i] = point
	}
	return result, nil
}

func parseSmallInt(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 4 {
		return 0, errBad
	}
	result := 0
	for _, char := range value {
		if char < '0' || char > '9' {
			return 0, errBad
		}
		result = result*10 + int(char-'0')
	}
	return result, nil
}

func secureInt(limit int) (int, error) {
	if limit <= 0 {
		return 0, errBad
	}
	value, err := rand.Int(rand.Reader, big.NewInt(int64(limit)))
	if err != nil {
		return 0, err
	}
	return int(value.Int64()), nil
}

func randomToken(size int) (string, error) {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func clientHash(client string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(client)))
	return hex.EncodeToString(sum[:16])
}
