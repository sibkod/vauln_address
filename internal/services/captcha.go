package services

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// CaptchaService issues and verifies one-time SVG captcha challenges.
// Answers live only in memory with a short TTL; no external dependencies.
type CaptchaService struct {
	mu      sync.Mutex
	answers map[string]captchaEntry
	ttl     time.Duration
	codeLen int
	lastGC  time.Time
}

type captchaEntry struct {
	answer    string
	expiresAt time.Time
}

// captchaAlphabet excludes ambiguous characters (0/O, 1/I/L).
const captchaAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

func NewCaptchaService() *CaptchaService {
	return &CaptchaService{
		answers: make(map[string]captchaEntry),
		ttl:     10 * time.Minute,
		codeLen: 5,
	}
}

func randInt(max int64) int {
	n, err := rand.Int(rand.Reader, big.NewInt(max))
	if err != nil {
		return 0
	}
	return int(n.Int64())
}

// Generate creates a challenge and returns its ID plus an SVG data URI.
func (s *CaptchaService) Generate() (string, string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if time.Since(s.lastGC) > time.Minute {
		now := time.Now()
		for id, e := range s.answers {
			if now.After(e.expiresAt) {
				delete(s.answers, id)
			}
		}
		s.lastGC = now
	}

	var code strings.Builder
	for i := 0; i < s.codeLen; i++ {
		code.WriteByte(captchaAlphabet[randInt(int64(len(captchaAlphabet)))])
	}

	id := uuid.NewString()
	s.answers[id] = captchaEntry{
		answer:    code.String(),
		expiresAt: time.Now().Add(s.ttl),
	}

	svg := renderCaptchaSVG(code.String())
	dataURI := "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))
	return id, dataURI
}

// Verify checks the answer and consumes the challenge (one-time use).
func (s *CaptchaService) Verify(id, answer string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.answers[id]
	if !ok {
		return false
	}
	delete(s.answers, id) // captcha is always single-use
	if time.Now().After(entry.expiresAt) {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(answer), entry.answer)
}

// Answer returns the expected answer for a challenge without consuming it.
// Intended for integration tests; the service object is never exposed over HTTP.
func (s *CaptchaService) Answer(id string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.answers[id]
	return entry.answer, ok
}

// renderCaptchaSVG draws the code with jittered glyphs and noise so simple
// OCR/bots fail while humans read it easily.
func renderCaptchaSVG(code string) string {
	var b strings.Builder
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="220" height="64" viewBox="0 0 220 64">`)
	b.WriteString(`<rect width="220" height="64" rx="8" fill="#0c111b"/>`)

	// noise lines
	palette := []string{"#2a3548", "#3b5fcf", "#667eea", "#4c5a7a"}
	for i := 0; i < 6; i++ {
		x1, y1 := randInt(220), randInt(64)
		x2, y2 := randInt(220), randInt(64)
		b.WriteString(fmt.Sprintf(`<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="%s" stroke-width="1.2" opacity="0.6"/>`,
			x1, y1, x2, y2, palette[randInt(int64(len(palette)))]))
	}
	// noise dots
	for i := 0; i < 24; i++ {
		b.WriteString(fmt.Sprintf(`<circle cx="%d" cy="%d" r="1" fill="%s" opacity="0.7"/>`,
			randInt(220), randInt(64), palette[randInt(int64(len(palette)))]))
	}

	for i, ch := range code {
		x := 22 + i*38 + randInt(8)
		y := 38 + randInt(12) - 6
		rot := randInt(41) - 20
		color := []string{"#e7ecf5", "#a8bfff", "#8fd3ff", "#ffd28f", "#c7d2e8"}[randInt(5)]
		b.WriteString(fmt.Sprintf(`<text x="%d" y="%d" transform="rotate(%d %d %d)" font-family="monospace" font-size="30" font-weight="700" fill="%s">%c</text>`,
			x, y, rot, x, y, color, ch))
	}

	b.WriteString(`</svg>`)
	return b.String()
}
