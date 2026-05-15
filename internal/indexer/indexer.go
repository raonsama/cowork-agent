// Package indexer walks a project directory and populates the SQLite
// symbol index used for context-aware code search during agent execution.
package indexer

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Progress is reported during indexing.
type Progress struct {
	Total   int
	Done    int
	Current string
	Symbols int
	Error   error
}

// Indexer walks a project directory and populates the SQLite index.
type Indexer struct {
	db            *DB
	ignoredDirs   map[string]bool
	supportedExts map[string]bool
	workerCount   int
	progressCh    chan Progress
}

// NewIndexer creates a ready-to-use Indexer.
func NewIndexer(db *DB, ignoredDirs, supportedExts []string, workers int) *Indexer {
	ignored := make(map[string]bool, len(ignoredDirs))
	for _, d := range ignoredDirs {
		ignored[d] = true
	}
	exts := make(map[string]bool, len(supportedExts))
	for _, e := range supportedExts {
		exts[e] = true
	}
	if workers <= 0 {
		workers = 2 // conservative for mobile
	}
	return &Indexer{
		db:            db,
		ignoredDirs:   ignored,
		supportedExts: exts,
		workerCount:   workers,
		progressCh:    make(chan Progress, 64),
	}
}

// Progress returns the channel on which index progress is reported.
func (idx *Indexer) Progress() <-chan Progress {
	return idx.progressCh
}

// IndexProject walks root and indexes all supported files.
func (idx *Indexer) IndexProject(root string) error {
	defer close(idx.progressCh)

	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible paths
		}
		if d.IsDir() {
			if idx.ignoredDirs[d.Name()] || strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if idx.supportedExts[filepath.Ext(path)] {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk: %w", err)
	}

	total := len(files)
	idx.progressCh <- Progress{Total: total, Done: 0}

	// Worker pool
	type job struct{ path string }
	jobCh := make(chan job, total)
	for _, f := range files {
		jobCh <- job{f}
	}
	close(jobCh)

	var (
		mu      sync.Mutex
		done    int
		symbols int
	)

	var wg sync.WaitGroup
	for i := 0; i < idx.workerCount; i++ {
		wg.Go(func() {
			for j := range jobCh {
				sym, err := idx.indexFile(j.path)
				mu.Lock()
				done++
				symbols += sym
				idx.progressCh <- Progress{
					Total:   total,
					Done:    done,
					Current: j.path,
					Symbols: symbols,
					Error:   err,
				}
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	return nil
}

// indexFile parses a single file and writes its symbols to the DB.
func (idx *Indexer) indexFile(path string) (int, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}

	hash, err := hashFile(path)
	if err != nil {
		return 0, err
	}

	modTime := info.ModTime().Unix()
	needsReindex, existingID := idx.db.FileNeedsReindex(path, hash, modTime)
	if !needsReindex {
		return 0, nil // up to date
	}

	fr := &FileRecord{
		Path:      path,
		Ext:       filepath.Ext(path),
		Size:      info.Size(),
		ModTime:   modTime,
		Hash:      hash,
		IndexedAt: time.Now().Unix(),
	}

	fileID, err := idx.db.UpsertFile(fr)
	if err != nil {
		return 0, err
	}

	// If file existed before, clean old symbols
	if existingID != 0 {
		_ = idx.db.DeleteSymbolsForFile(fileID)
	}

	// Parse symbols
	symbols, err := parseSymbols(path, fileID, filepath.Ext(path))
	if err != nil {
		return 0, err
	}

	for _, s := range symbols {
		_ = idx.db.InsertSymbol(s)
	}
	return len(symbols), nil
}

// hashFile returns the SHA-256 hex digest of a file.
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// ─────────────────────────────────────────────────────────
// Language parsers (regex-based, zero-dependency)
// ─────────────────────────────────────────────────────────

var parsers = map[string]func(string, int64) ([]*SymbolRecord, error){
	".go":  parseGo,
	".py":  parsePython,
	".ts":  parseJS,
	".js":  parseJS,
	".lua": parseLua,
	".rs":  parseRust,
}

func parseSymbols(path string, fileID int64, ext string) ([]*SymbolRecord, error) {
	parser, ok := parsers[ext]
	if !ok {
		return nil, nil
	}
	syms, err := parser(path, fileID)
	return syms, err
}

// readLines reads a file into a string slice.
func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines, sc.Err()
}

// extractBody grabs up to 30 lines starting at lineStart.
func extractBody(lines []string, start, end int) string {
	if start < 0 {
		start = 0
	}
	if end > len(lines) {
		end = len(lines)
	}
	if end-start > 30 {
		end = start + 30
	}
	return strings.Join(lines[start:end], "\n")
}

// ── Go parser ──

var (
	reGoFunc  = regexp.MustCompile(`^func\s+(\([^)]+\)\s+)?(\w+)\s*\(`)
	reGoType  = regexp.MustCompile(`^type\s+(\w+)\s+`)
	reGoConst = regexp.MustCompile(`^\s*(\w+)\s*=`)
)

func parseGo(path string, fileID int64) ([]*SymbolRecord, error) {
	lines, err := readLines(path)
	if err != nil {
		return nil, err
	}

	var syms []*SymbolRecord
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if m := reGoFunc.FindStringSubmatch(trimmed); m != nil {
			name := m[2]
			if name == "" {
				continue
			}
			end := findClosingBrace(lines, i)
			syms = append(syms, &SymbolRecord{
				FileID:    fileID,
				Name:      name,
				Kind:      "func",
				LineStart: i + 1,
				LineEnd:   end + 1,
				Signature: strings.TrimSpace(line),
				Body:      extractBody(lines, i, end+1),
			})
		} else if m := reGoType.FindStringSubmatch(trimmed); m != nil {
			syms = append(syms, &SymbolRecord{
				FileID:    fileID,
				Name:      m[1],
				Kind:      "type",
				LineStart: i + 1,
				LineEnd:   i + 1,
				Signature: strings.TrimSpace(line),
				Body:      strings.TrimSpace(line),
			})
		}
	}
	return syms, nil
}

// findClosingBrace scans forward to find the matching } for a function body.
func findClosingBrace(lines []string, start int) int {
	depth := 0
	for i := start; i < len(lines) && i < start+200; i++ {
		for _, c := range lines[i] {
			if c == '{' {
				depth++
			} else if c == '}' {
				depth--
				if depth == 0 {
					return i
				}
			}
		}
	}
	return start
}

// ── Python parser ──

var (
	rePyFunc  = regexp.MustCompile(`^(def|async def)\s+(\w+)\s*\(`)
	rePyClass = regexp.MustCompile(`^class\s+(\w+)`)
)

func parsePython(path string, fileID int64) ([]*SymbolRecord, error) {
	lines, err := readLines(path)
	if err != nil {
		return nil, err
	}
	var syms []*SymbolRecord
	for i, line := range lines {
		if m := rePyFunc.FindStringSubmatch(line); m != nil {
			syms = append(syms, &SymbolRecord{
				FileID:    fileID,
				Name:      m[2],
				Kind:      "func",
				LineStart: i + 1,
				LineEnd:   i + 1,
				Signature: strings.TrimSpace(line),
				Body:      extractBody(lines, i, i+20),
			})
		} else if m := rePyClass.FindStringSubmatch(line); m != nil {
			syms = append(syms, &SymbolRecord{
				FileID:    fileID,
				Name:      m[1],
				Kind:      "class",
				LineStart: i + 1,
				LineEnd:   i + 1,
				Signature: strings.TrimSpace(line),
				Body:      extractBody(lines, i, i+5),
			})
		}
	}
	return syms, nil
}

// ── JS/TS parser ──

var (
	reJSFunc  = regexp.MustCompile(`(?:export\s+)?(?:async\s+)?function\s+(\w+)\s*\(`)
	reJSArrow = regexp.MustCompile(`(?:export\s+)?(?:const|let|var)\s+(\w+)\s*=\s*(?:async\s+)?\(`)
	reJSClass = regexp.MustCompile(`(?:export\s+)?class\s+(\w+)`)
)

func parseJS(path string, fileID int64) ([]*SymbolRecord, error) {
	lines, err := readLines(path)
	if err != nil {
		return nil, err
	}
	var syms []*SymbolRecord
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		var name, kind string
		if m := reJSFunc.FindStringSubmatch(trimmed); m != nil {
			name, kind = m[1], "func"
		} else if m := reJSArrow.FindStringSubmatch(trimmed); m != nil {
			name, kind = m[1], "func"
		} else if m := reJSClass.FindStringSubmatch(trimmed); m != nil {
			name, kind = m[1], "class"
		}
		if name != "" {
			syms = append(syms, &SymbolRecord{
				FileID:    fileID,
				Name:      name,
				Kind:      kind,
				LineStart: i + 1,
				LineEnd:   i + 1,
				Signature: trimmed,
				Body:      extractBody(lines, i, i+20),
			})
		}
	}
	return syms, nil
}

// ── Lua parser ──

var reLuaFunc = regexp.MustCompile(`(?:local\s+)?function\s+([\w.:]+)\s*\(`)

func parseLua(path string, fileID int64) ([]*SymbolRecord, error) {
	lines, err := readLines(path)
	if err != nil {
		return nil, err
	}
	var syms []*SymbolRecord
	for i, line := range lines {
		if m := reLuaFunc.FindStringSubmatch(line); m != nil {
			syms = append(syms, &SymbolRecord{
				FileID:    fileID,
				Name:      m[1],
				Kind:      "func",
				LineStart: i + 1,
				LineEnd:   i + 1,
				Signature: strings.TrimSpace(line),
				Body:      extractBody(lines, i, i+20),
			})
		}
	}
	return syms, nil
}

// ── Rust parser ──

var (
	reRustFn     = regexp.MustCompile(`(?:pub\s+)?(?:async\s+)?fn\s+(\w+)\s*(?:<[^>]*>)?\s*\(`)
	reRustStruct = regexp.MustCompile(`(?:pub\s+)?struct\s+(\w+)`)
	reRustEnum   = regexp.MustCompile(`(?:pub\s+)?enum\s+(\w+)`)
)

func parseRust(path string, fileID int64) ([]*SymbolRecord, error) {
	lines, err := readLines(path)
	if err != nil {
		return nil, err
	}
	var syms []*SymbolRecord
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		var name, kind string
		if m := reRustFn.FindStringSubmatch(trimmed); m != nil {
			name, kind = m[1], "func"
		} else if m := reRustStruct.FindStringSubmatch(trimmed); m != nil {
			name, kind = m[1], "struct"
		} else if m := reRustEnum.FindStringSubmatch(trimmed); m != nil {
			name, kind = m[1], "enum"
		}
		if name != "" {
			end := findClosingBrace(lines, i)
			syms = append(syms, &SymbolRecord{
				FileID:    fileID,
				Name:      name,
				Kind:      kind,
				LineStart: i + 1,
				LineEnd:   end + 1,
				Signature: trimmed,
				Body:      extractBody(lines, i, end+1),
			})
		}
	}
	return syms, nil
}
