package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

type params struct {
	Argon2Variant string
	Argon2Version int
	Memory        uint32
	Iterations    uint32
	Parallelism   uint8
	SaltLen       uint32
	KeyLen        uint32
}

type hashData struct {
	Params *params
	Salt   []byte
	Key    []byte
}

var (
	ErrInvalidHash               = errors.New("hash: invalid format")
	ErrPasswordMismatch          = errors.New("hash: password does not match")
	ErrHashingParametersMismatch = errors.New("hash: invalid parameters")
)

// RFC 9106
// https://www.rfc-editor.org/info/rfc9106/#name-recommendations
//
// Argon2id Recommended Settings
//
// Recommendation 1: Default / High Security (2 GiB)
// Secure against side-channels, maximizes cost for brute-force hardware.
// Iterations  = 1
// Memory      = 2 * 1024 * 1024 // 2 GiB in KiB
// Parallelism = 4               // Usually set to number of available cores
//
// Recommendation 2: Memory-Constrained (64 MiB)
// Suggested for environments where RAM is limited.
// Iterations  = 3
// Memory      = 64 * 1024      // 64 MiB in KiB
// Parallelism = 2

func getDefaultParams() *params {
	return &params{
		Argon2Variant: "argon2id",
		Argon2Version: argon2.Version,
		Memory:        64 * 1024,
		Iterations:    3,
		Parallelism:   2,
		SaltLen:       16,
		KeyLen:        32,
	}
}

// https://github.com/P-H-C/phc-winner-argon2#command-line-utility
// type - version - memory - iterations - parallelism - salt - key
// $argon2id$v=19$m=65536,t=2,p=4$c29tZXNhbHQ$RdescudvJCsgt3ub+b+dWRWJTmaaJObG
func GenerateFromPassword(password []byte) (string, error) {
	p := getDefaultParams()

	salt, err := getSalt(p.SaltLen)
	if err != nil {
		return "", err
	}

	key := argon2.IDKey(password, salt, p.Iterations, p.Memory, p.Parallelism, p.KeyLen)

	hash := fmt.Sprintf(
		"$%s$v=%d$m=%d,t=%d,p=%d$%s$%s",
		p.Argon2Variant, p.Argon2Version, p.Memory, p.Iterations, p.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	)

	return hash, nil
}

func CompareHashAndPassword(hashedPassword string, password []byte) error {
	h, err := parseHash(hashedPassword)

	if err != nil {
		return err
	}

	if err := h.Params.isValid(); err != nil {
		return ErrHashingParametersMismatch
	}

	otherKey := argon2.IDKey(password, h.Salt, h.Params.Iterations, h.Params.Memory, h.Params.Parallelism, h.Params.KeyLen)

	if subtle.ConstantTimeCompare(h.Key, otherKey) != 1 {
		return ErrPasswordMismatch
	}

	return nil
}

func parseHash(hash string) (*hashData, error) {
	vars := strings.Split(hash, "$")

	if len(vars) != 6 || vars[0] != "" {
		return nil, ErrInvalidHash
	}

	var p params
	p.Argon2Variant = vars[1]

	if _, err := fmt.Sscanf(vars[2], "v=%d", &p.Argon2Version); err != nil {
		return nil, fmt.Errorf("%w: invalid hash version: %v", ErrInvalidHash, err)
	}

	if _, err := fmt.Sscanf(vars[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Iterations, &p.Parallelism); err != nil {
		return nil, fmt.Errorf("%w: invalid hash parameters: %v", ErrInvalidHash, err)
	}

	// base64 decoder will ignore new line characters (\r and \n),
	// check these chars in salt and key parts before decoding
	if strings.ContainsAny(vars[4], "\r\n") || strings.ContainsAny(vars[5], "\r\n") {
		return nil, fmt.Errorf("%w: invalid hash parameters: unexpected symbols", ErrInvalidHash)
	}

	salt, err := base64.RawStdEncoding.DecodeString(vars[4])
	if err != nil {
		return nil, fmt.Errorf("%w: invalid salt encoding: %v", ErrInvalidHash, err)
	}
	p.SaltLen = uint32(len(salt))

	key, err := base64.RawStdEncoding.DecodeString(vars[5])
	if err != nil {
		return nil, fmt.Errorf("%w: invalid key encoding: %v", ErrInvalidHash, err)
	}
	p.KeyLen = uint32(len(key))

	return &hashData{
		Params: &p,
		Salt:   salt,
		Key:    key,
	}, nil
}

func (hashParams *params) isValid() error {
	p := getDefaultParams()

	if hashParams.Argon2Variant != p.Argon2Variant {
		return errors.New("divergent argon2 algorithm variant")
	}

	if hashParams.Argon2Version != p.Argon2Version {
		return errors.New("divergent argon2 library version")
	}

	if hashParams.Iterations != p.Iterations || hashParams.Memory != p.Memory || hashParams.Parallelism != p.Parallelism {
		return errors.New("divergent iterations, memory or parallelism parameters")
	}

	if hashParams.SaltLen != p.SaltLen {
		return errors.New("divergent salt parameters")
	}

	if hashParams.KeyLen != p.KeyLen {
		return errors.New("divergent key parameters")
	}

	return nil
}

func getSalt(saltLen uint32) ([]byte, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		// in reality rand.Read will crash the program irrecoverably
		// https://pkg.go.dev/crypto/rand@go1.26.4#Read
		return salt, err
	}
	return salt, nil
}
