package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const defaultImageConcurrency = 5

func configuredImageConcurrency() (int, error) {
	raw := strings.TrimSpace(os.Getenv("MULTICA_IMAGE_MAX_CONCURRENT"))
	if raw == "" {
		return defaultImageConcurrency, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || value > 20 {
		return 0, fmt.Errorf("MULTICA_IMAGE_MAX_CONCURRENT must be between 1 and 20")
	}
	return value, nil
}

func acquireGlobalImageSlot(ctx context.Context, endpoint, apiKey string) (func() error, error) {
	limit, err := configuredImageConcurrency()
	if err != nil {
		return nil, err
	}
	root := strings.TrimSpace(os.Getenv("MULTICA_IMAGE_SLOT_DIR"))
	if root == "" {
		root = filepath.Join(os.TempDir(), "multica-image-slots")
	}
	digest := sha256.Sum256([]byte(endpoint + "\x00" + apiKey))
	providerDir := filepath.Join(root, hex.EncodeToString(digest[:8]))
	if err := os.MkdirAll(providerDir, 0o700); err != nil {
		return nil, fmt.Errorf("create image slot directory: %w", err)
	}

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		for index := 0; index < limit; index++ {
			path := filepath.Join(providerDir, fmt.Sprintf("slot-%02d.lock", index+1))
			lock, acquired, lockErr := tryLockImageSlot(path)
			if lockErr != nil {
				return nil, lockErr
			}
			if acquired {
				return lock.release, nil
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}
