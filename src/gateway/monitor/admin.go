package monitor

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Password hashing: PBKDF2-HMAC-SHA256 from the standard library (crypto/pbkdf2,
// Go 1.24+). Chosen over bcrypt so the gateway gets no extra dependency (it
// already adds SQLite) and because a single administrator with a long random
// password is well served by 600,000 iterations (OWASP 2023 figure for
// PBKDF2-HMAC-SHA256). bcrypt would also work; the stored format is
// self-describing, so it can be swapped later.
//
// File format (one line): user:pbkdf2-sha256$<iterations>$<salt b64>$<hash b64>

// DefaultIterations is the cost used by HashPassword callers.
const DefaultIterations = 600000

const minIterations = 1000 // a floor for sanity; tests use the floor, production uses the default

// Credentials is the single administrator.
type Credentials struct {
	User string
	iter int
	salt []byte
	hash []byte
}

// HashPassword returns the credential line value "pbkdf2-sha256$iter$salt$hash".
func HashPassword(password string, iter int) (string, error) {
	if iter < minIterations {
		return "", fmt.Errorf("iterations must be at least %d", minIterations)
	}
	if len(password) < 8 {
		return "", errors.New("a senha precisa ter pelo menos 8 caracteres")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	h, err := pbkdf2.Key(sha256.New, password, salt, iter, 32)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", iter, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(h)), nil
}

// ParseCredentials parses "user:pbkdf2-sha256$iter$salt$hash".
func ParseCredentials(line string) (*Credentials, error) {
	line = strings.TrimSpace(line)
	user, rest, ok := strings.Cut(line, ":")
	if !ok || user == "" || len(user) > 64 {
		return nil, errors.New("formato inválido: esperado usuario:pbkdf2-sha256$iter$salt$hash")
	}
	parts := strings.Split(rest, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return nil, errors.New("formato de hash não suportado (esperado pbkdf2-sha256)")
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < minIterations || iter > 50_000_000 {
		return nil, errors.New("número de iterações inválido")
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[2])
	hash, err2 := base64.RawStdEncoding.DecodeString(parts[3])
	if err1 != nil || err2 != nil || len(salt) < 8 || len(hash) != 32 {
		return nil, errors.New("salt ou hash inválido")
	}
	return &Credentials{User: user, iter: iter, salt: salt, hash: hash}, nil
}

// LoadAdminFile reads the credentials file. On Unix it must not be readable by
// group or others (0600), otherwise the start is refused.
func LoadAdminFile(path string) (*Credentials, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("--admin-file: %w", err)
	}
	if runtime.GOOS != "windows" && fi.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("--admin-file %s tem permissão %v: use 0600", path, fi.Mode().Perm())
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("--admin-file: %w", err)
	}
	return ParseCredentials(string(b))
}

// Verify checks user and password in constant time. The hash is always
// computed, so the answer time does not tell whether the user exists.
func (c *Credentials) Verify(user, password string) bool {
	h, err := pbkdf2.Key(sha256.New, password, c.salt, c.iter, 32)
	if err != nil {
		return false
	}
	okHash := subtle.ConstantTimeCompare(h, c.hash) == 1
	// Compare digests of equal length: ConstantTimeCompare returns at once on
	// a length mismatch, which would tell the length of the user name.
	gu, wu := sha256.Sum256([]byte(user)), sha256.Sum256([]byte(c.User))
	okUser := subtle.ConstantTimeCompare(gu[:], wu[:]) == 1
	return okHash && okUser
}

func constEq(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }

// ---- sessions ----------------------------------------------------------------

const (
	sessionTTL  = 8 * time.Hour
	maxSessions = 8
	cookieName  = "mcpgw_session"
	loginWindow = 15 * time.Minute
	loginMaxIP  = 5
	// Fase 9.5: no global ceiling any more (anonymous failures from many IPs
	// used to lock the administrator out). Each IP gets loginMaxIP attempts;
	// after that it is locked out for loginWindow, doubling at each new
	// lockout (exponential backoff) up to loginMaxLock.
	loginMaxLock    = 24 * time.Hour
	loginMaxTracked = 4096 // above this, idle entries are swept
	verifySlots     = 2    // concurrent PBKDF2 evaluations
)

type session struct {
	user string
	csrf string
	exp  time.Time
}

type sessions struct {
	mu sync.Mutex
	m  map[string]*session
}

func newToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("monitor: crypto/rand failed: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func (s *sessions) create(user string, now time.Time) (token string, sess *session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = map[string]*session{}
	}
	for k, v := range s.m {
		if !v.exp.After(now) {
			delete(s.m, k)
		}
	}
	// A single administrator: when the cap is reached the oldest goes.
	for len(s.m) >= maxSessions {
		var oldK string
		var oldT time.Time
		for k, v := range s.m {
			if oldK == "" || v.exp.Before(oldT) {
				oldK, oldT = k, v.exp
			}
		}
		delete(s.m, oldK)
	}
	token = newToken()
	sess = &session{user: user, csrf: newToken(), exp: now.Add(sessionTTL)}
	s.m[token] = sess
	return token, sess
}

func (s *sessions) get(token string, now time.Time) *session {
	if token == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.m[token]
	if v == nil {
		return nil
	}
	if !v.exp.After(now) {
		delete(s.m, token)
		return nil
	}
	return v
}

func (s *sessions) drop(token string) {
	s.mu.Lock()
	delete(s.m, token)
	s.mu.Unlock()
}

// ---- login rate limit ----------------------------------------------------------

// ipAttempts is the login record of one client IP.
type ipAttempts struct {
	fails   []time.Time // failures inside the window
	pending int         // attempts being evaluated right now (reserved)
	until   time.Time   // locked out until
	strikes int         // lockouts so far (exponential backoff)
	last    time.Time   // last activity
}

// limiter bounds login attempts per IP. An attempt is reserved atomically
// (tryBegin) before the slow password check and counts against the limit while
// it runs, so parallel requests cannot all slip through before the first
// failure is recorded (Fase 9.5, M1).
type limiter struct {
	mu   sync.Mutex
	byIP map[string]*ipAttempts
}

func prune(v []time.Time, cut time.Time) []time.Time {
	i := 0
	for i < len(v) && v[i].Before(cut) {
		i++
	}
	return v[i:]
}

// tryBegin reserves one attempt for ip. When it returns false, retry is how
// long the client should wait.
func (l *limiter) tryBegin(ip string, now time.Time) (ok bool, retry time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.byIP == nil {
		l.byIP = map[string]*ipAttempts{}
	}
	if len(l.byIP) > loginMaxTracked {
		l.sweepLocked(now)
	}
	a := l.byIP[ip]
	if a == nil {
		a = &ipAttempts{}
		l.byIP[ip] = a
	}
	if now.Sub(a.last) > loginMaxLock {
		a.strikes = 0 // a quiet day forgives the old lockouts
	}
	a.last = now
	a.fails = prune(a.fails, now.Add(-loginWindow))
	if now.Before(a.until) {
		return false, a.until.Sub(now)
	}
	if len(a.fails)+a.pending >= loginMaxIP {
		return false, loginWindow
	}
	a.pending++
	return true, 0
}

// end closes a reserved attempt. success clears the record of the IP.
func (l *limiter) end(ip string, now time.Time, success bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	a := l.byIP[ip]
	if a == nil {
		return
	}
	if a.pending > 0 {
		a.pending--
	}
	a.last = now
	if success {
		delete(l.byIP, ip)
		return
	}
	a.fails = append(prune(a.fails, now.Add(-loginWindow)), now)
	if len(a.fails) >= loginMaxIP {
		a.strikes++
		lock := loginWindow << (a.strikes - 1)
		if lock > loginMaxLock || lock <= 0 {
			lock = loginMaxLock
		}
		a.until = now.Add(lock)
		a.fails = nil
	}
}

// abort gives a reserved attempt back without counting it (the password was
// never evaluated).
func (l *limiter) abort(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if a := l.byIP[ip]; a != nil && a.pending > 0 {
		a.pending--
	}
}

// sweepLocked drops the entries that carry no state worth keeping.
func (l *limiter) sweepLocked(now time.Time) {
	cut := now.Add(-loginWindow)
	for ip, a := range l.byIP {
		idle := a.pending == 0 && !now.Before(a.until) && len(prune(a.fails, cut)) == 0
		if idle && (a.strikes == 0 || now.Sub(a.last) > loginMaxLock) {
			delete(l.byIP, ip)
		}
	}
}

// tracked is the number of IPs the limiter remembers (tests).
func (l *limiter) tracked() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.byIP)
}
