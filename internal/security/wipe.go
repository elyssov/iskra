package security

import (
	"crypto/rand"
	"log"
	"os"
	"path/filepath"
)

// WipeAll securely destroys all data in the Iskra data directory.
// Each file is overwritten with random data before deletion.
// This makes forensic recovery impossible.
func WipeAll(dataDir string) error {
	log.Println("[PANIC] Wiping all data in", dataDir)

	return filepath.Walk(dataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip inaccessible files
		}
		if info.IsDir() {
			return nil
		}
		if err := SecureDeleteFile(path); err != nil {
			log.Printf("[PANIC] Failed to wipe %s: %v", path, err)
		}
		return nil
	})
}

// SecureDeleteFile overwrites a file with random data, syncs, then removes it.
// Overwrite is critical: deleted files can be recovered, overwritten cannot.
func SecureDeleteFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	size := info.Size()
	if size > 0 {
		f, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err != nil {
			// Can't open for write — just delete
			return os.Remove(path)
		}

		// Overwrite with random data (3 passes for extra safety)
		buf := make([]byte, min(size, 64*1024)) // max 64KB buffer
		for pass := 0; pass < 3; pass++ {
			f.Seek(0, 0)
			remaining := size
			for remaining > 0 {
				n := min(remaining, int64(len(buf)))
				rand.Read(buf[:n])
				f.Write(buf[:n])
				remaining -= n
			}
			f.Sync()
		}
		f.Close()
	}

	return os.Remove(path)
}

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
