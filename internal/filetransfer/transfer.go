// Package filetransfer handles chunked encrypted file transfer over mesh.
//
// Files are compressed (zip), split into chunks < 900KB, sent as individual
// encrypted messages, and reassembled on the receiving end.
//
// Limits: 10MB max file size, 7-day TTL for chunks, HopTTL=5.
package filetransfer

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	MaxFileSize   = 10 * 1024 * 1024 // 10 MB
	ChunkSize     = 900 * 1024       // 900 KB per chunk
	ChunkTTLDays  = 7                // 7 days TTL for file chunks
	ChunkHopTTL   = 5                // Fewer hops than text messages
	HeaderSize    = 16 + 2 + 2       // transferID(16) + chunkIndex(2) + totalChunks(2)
)

// ChunkHeader is prepended to each chunk payload.
type ChunkHeader struct {
	TransferID  [16]byte // Random ID linking all chunks of one file
	ChunkIndex  uint16   // 0-based
	TotalChunks uint16   // Total number of chunks
}

// FileMetadata is encoded in chunk 0's data prefix.
type FileMetadata struct {
	Filename string
	MimeType string
	FileSize int64
}

// Transfer tracks an in-progress file reception.
type Transfer struct {
	ID          string
	Filename    string
	MimeType    string
	FileSize    int64
	TotalChunks int
	Chunks      map[int][]byte // chunkIndex -> data
	CreatedAt   time.Time
}

// Manager handles file chunking and reassembly.
type Manager struct {
	mu        sync.Mutex
	pending   map[string]*Transfer // transferID hex -> transfer
	outputDir string               // where to save completed files
}

// NewManager creates a file transfer manager.
func NewManager(outputDir string) *Manager {
	os.MkdirAll(outputDir, 0700)
	return &Manager{
		pending:   make(map[string]*Transfer),
		outputDir: outputDir,
	}
}

// PrepareChunks takes a file, compresses it, and splits into chunk payloads.
// Each payload = ChunkHeader + data. Chunk 0 also has FileMetadata prefix.
// Returns payloads ready to be used as message.Payload.
func PrepareChunks(filePath string) ([][]byte, error) {
	// Read file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	if len(data) > MaxFileSize {
		return nil, fmt.Errorf("file too large: %d bytes (max %d)", len(data), MaxFileSize)
	}

	filename := filepath.Base(filePath)
	mimeType := detectMime(filename)

	// Compress
	compressed, err := compressZip(filename, data)
	if err != nil {
		return nil, fmt.Errorf("compress: %w", err)
	}

	// If compression made it bigger, use raw
	if len(compressed) >= len(data) {
		compressed = data
	}

	// Generate transfer ID
	var transferID [16]byte
	rand.Read(transferID[:])

	// Split into chunks
	totalChunks := (len(compressed) + ChunkSize - 1) / ChunkSize
	if totalChunks > 65535 {
		return nil, fmt.Errorf("too many chunks: %d", totalChunks)
	}

	// Encode metadata for chunk 0
	meta := encodeMetadata(filename, mimeType, int64(len(data)))

	var payloads [][]byte
	for i := 0; i < totalChunks; i++ {
		start := i * ChunkSize
		end := start + ChunkSize
		if end > len(compressed) {
			end = len(compressed)
		}
		chunkData := compressed[start:end]

		// Header
		header := ChunkHeader{
			TransferID:  transferID,
			ChunkIndex:  uint16(i),
			TotalChunks: uint16(totalChunks),
		}
		hdr := encodeHeader(header)

		var payload []byte
		if i == 0 {
			// Chunk 0: header + metadata + data
			payload = make([]byte, 0, len(hdr)+len(meta)+len(chunkData))
			payload = append(payload, hdr...)
			payload = append(payload, meta...)
			payload = append(payload, chunkData...)
		} else {
			// Other chunks: header + data
			payload = make([]byte, 0, len(hdr)+len(chunkData))
			payload = append(payload, hdr...)
			payload = append(payload, chunkData...)
		}
		payloads = append(payloads, payload)
	}

	log.Printf("[FileTransfer] Prepared %d chunks for %q (%d bytes -> %d compressed)",
		totalChunks, filename, len(data), len(compressed))
	return payloads, nil
}

// ReceiveChunk processes an incoming file chunk. Returns (filePath, true) when complete.
func (m *Manager) ReceiveChunk(payload []byte) (string, bool) {
	if len(payload) < HeaderSize {
		return "", false
	}

	header := decodeHeader(payload[:HeaderSize])
	idHex := hex.EncodeToString(header.TransferID[:])

	m.mu.Lock()
	defer m.mu.Unlock()

	t, exists := m.pending[idHex]
	if !exists {
		t = &Transfer{
			ID:          idHex,
			TotalChunks: int(header.TotalChunks),
			Chunks:      make(map[int][]byte),
			CreatedAt:   time.Now(),
		}
		m.pending[idHex] = t
	}

	chunkIdx := int(header.ChunkIndex)
	chunkData := payload[HeaderSize:]

	// Chunk 0 has metadata prefix
	if chunkIdx == 0 {
		meta, dataOffset := decodeMetadata(chunkData)
		t.Filename = meta.Filename
		t.MimeType = meta.MimeType
		t.FileSize = meta.FileSize
		chunkData = chunkData[dataOffset:]
	}

	t.Chunks[chunkIdx] = chunkData

	// Check if complete
	if len(t.Chunks) < t.TotalChunks {
		log.Printf("[FileTransfer] Chunk %d/%d for %s", chunkIdx+1, t.TotalChunks, idHex[:8])
		return "", false
	}

	// Assemble
	log.Printf("[FileTransfer] All %d chunks received for %q — assembling", t.TotalChunks, t.Filename)
	var assembled bytes.Buffer
	for i := 0; i < t.TotalChunks; i++ {
		assembled.Write(t.Chunks[i])
	}

	// Try decompress
	result, err := decompressZip(assembled.Bytes())
	if err != nil {
		// Not zipped — use raw
		result = assembled.Bytes()
	}

	// Save
	safeName := filepath.Base(t.Filename)
	if safeName == "" || safeName == "." {
		safeName = "file_" + idHex[:8]
	}
	outPath := filepath.Join(m.outputDir, safeName)
	if err := os.WriteFile(outPath, result, 0600); err != nil {
		log.Printf("[FileTransfer] Save error: %v", err)
		delete(m.pending, idHex)
		return "", false
	}

	delete(m.pending, idHex)
	log.Printf("[FileTransfer] Saved %q (%d bytes)", outPath, len(result))
	return outPath, true
}

// Cleanup removes stale incomplete transfers (older than 7 days).
func (m *Manager) Cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, t := range m.pending {
		if time.Since(t.CreatedAt) > 7*24*time.Hour {
			delete(m.pending, id)
			log.Printf("[FileTransfer] Cleaned up stale transfer %s", id[:8])
		}
	}
}

// --- Encoding helpers ---

func encodeHeader(h ChunkHeader) []byte {
	buf := make([]byte, HeaderSize)
	copy(buf[:16], h.TransferID[:])
	binary.BigEndian.PutUint16(buf[16:18], h.ChunkIndex)
	binary.BigEndian.PutUint16(buf[18:20], h.TotalChunks)
	return buf
}

func decodeHeader(buf []byte) ChunkHeader {
	var h ChunkHeader
	copy(h.TransferID[:], buf[:16])
	h.ChunkIndex = binary.BigEndian.Uint16(buf[16:18])
	h.TotalChunks = binary.BigEndian.Uint16(buf[18:20])
	return h
}

func encodeMetadata(filename, mimeType string, fileSize int64) []byte {
	// Format: filenameLen(2) + filename + mimeLen(2) + mime + fileSize(8)
	buf := make([]byte, 0, 2+len(filename)+2+len(mimeType)+8)
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, uint16(len(filename)))
	buf = append(buf, b...)
	buf = append(buf, []byte(filename)...)
	binary.BigEndian.PutUint16(b, uint16(len(mimeType)))
	buf = append(buf, b...)
	buf = append(buf, []byte(mimeType)...)
	sz := make([]byte, 8)
	binary.BigEndian.PutUint64(sz, uint64(fileSize))
	buf = append(buf, sz...)
	return buf
}

func decodeMetadata(data []byte) (FileMetadata, int) {
	var m FileMetadata
	offset := 0

	if len(data) < 2 {
		return m, 0
	}
	fnLen := int(binary.BigEndian.Uint16(data[offset:]))
	offset += 2
	if offset+fnLen > len(data) {
		return m, 0
	}
	m.Filename = string(data[offset : offset+fnLen])
	offset += fnLen

	if offset+2 > len(data) {
		return m, offset
	}
	mtLen := int(binary.BigEndian.Uint16(data[offset:]))
	offset += 2
	if offset+mtLen > len(data) {
		return m, offset
	}
	m.MimeType = string(data[offset : offset+mtLen])
	offset += mtLen

	if offset+8 > len(data) {
		return m, offset
	}
	m.FileSize = int64(binary.BigEndian.Uint64(data[offset:]))
	offset += 8

	return m, offset
}

func detectMime(filename string) string {
	ext := filepath.Ext(filename)
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".mp4":
		return "video/mp4"
	case ".pdf":
		return "application/pdf"
	case ".doc", ".docx":
		return "application/msword"
	case ".txt":
		return "text/plain"
	default:
		return "application/octet-stream"
	}
}

func compressZip(filename string, data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, err := w.Create(filename)
	if err != nil {
		return nil, err
	}
	f.Write(data)
	w.Close()
	return buf.Bytes(), nil
}

func decompressZip(data []byte) ([]byte, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	if len(r.File) == 0 {
		return nil, fmt.Errorf("empty zip")
	}
	f, err := r.File[0].Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}
