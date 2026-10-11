// Package tynelapi is compiled INSIDE the pinned TypeScript module by a
// source overlay. Only Tynel-owned values cross its exported API boundary.
package tynelapi

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/microsoft/typescript-go/internal/ast"
	"github.com/microsoft/typescript-go/internal/bundled"
	"github.com/microsoft/typescript-go/internal/compiler"
	"github.com/microsoft/typescript-go/internal/core"
	"github.com/microsoft/typescript-go/internal/scanner"
	"github.com/microsoft/typescript-go/internal/tsoptions"
	"github.com/microsoft/typescript-go/internal/tspath"
	"github.com/microsoft/typescript-go/internal/vfs/vfstest"
)

const UpstreamCommit = "1e4744d68260a7cb91b62b12edc3f6a2187faaf1"
const Protocol = "tynel.tsgo-probe/1"
const MaxSourceBytes = 64 << 10

var ErrClosed = errors.New("TY1101: frontend session is closed")
var ErrSourceDiagnostics = errors.New("TY1102: type query blocked by source diagnostics")

// Position uses a zero-based byte offset, and one-based line/UTF-16 column.
// It describes this source snapshot, never a location in a later edited file.
type Position struct {
	ByteOffset  int `json:"byteOffset"`
	Line        int `json:"line"`
	UTF16Column int `json:"utf16Column"`
}

type Diagnostic struct {
	Code    int32    `json:"code"`
	Message string   `json:"message"`
	File    string   `json:"file"`
	Start   Position `json:"start"`
	End     Position `json:"end"`
}

type TypeFact struct {
	SourceSHA256 string   `json:"sourceSHA256"`
	NodeKind     string   `json:"nodeKind"`
	Text         string   `json:"text"`
	Display      string   `json:"display"`
	Start        Position `json:"start"`
	End          Position `json:"end"`
}

type Snapshot struct {
	Protocol       string       `json:"protocol"`
	UpstreamCommit string       `json:"upstreamCommit"`
	SourceSHA256   string       `json:"sourceSHA256"`
	Diagnostics    []Diagnostic `json:"diagnostics"`
}

// Session owns an immutable single-file program. Its mutex serializes queries
// and Close. It deliberately exposes no checker, AST, symbol, or type handle.
// This is an internal-access probe, not the final project resolver/facade.
type Session struct {
	mu       sync.Mutex
	program  *compiler.Program
	file     *ast.SourceFile
	text     string
	snapshot Snapshot
	scalar   bool
}

func New(source string) (*Session, error) { return newSession(source, false) }

// NewScalar enables the explicitly limited scalar console capability used by
// the native scalar slice. The query-only New API keeps its previous environment.
func NewScalar(source string) (*Session, error) { return newSession(source, true) }

const printDeclarationFile = "/tynel/native-print.d.ts"
const printDeclaration = "declare const console: { readonly log: (value: number | boolean) => void };\n"

func newSession(source string, scalar bool) (*Session, error) {
	if len(source) > MaxSourceBytes || !utf8.ValidString(source) {
		return nil, errors.New("TY1100: source must be valid UTF-8, at most 64 KiB")
	}
	const cwd = "/tynel"
	const name = "/tynel/probe.ts"
	// The upstream's in-memory test FS keeps the probe hermetic: imports can
	// only resolve within this snapshot and the pinned bundled declarations.
	files := map[string]string{name: source}
	roots := []string{name}
	if scalar {
		files[printDeclarationFile] = printDeclaration
		roots = append(roots, printDeclarationFile)
	}
	fs := bundled.WrapFS(vfstest.FromMap(files, true))
	host := compiler.NewCompilerHost(cwd, fs, bundled.LibPath(), nil, nil)
	opts := &core.CompilerOptions{Strict: core.TSTrue, NoEmit: core.TSTrue, SkipLibCheck: core.TSTrue, Lib: []string{"lib.es5.d.ts"}}
	config := tsoptions.NewParsedCommandLine(opts, roots, tspath.ComparePathsOptions{CurrentDirectory: cwd, UseCaseSensitiveFileNames: true})
	p := compiler.NewProgram(compiler.ProgramOptions{Host: host, Config: config, SingleThreaded: core.TSTrue})
	var file *ast.SourceFile
	for _, f := range p.SourceFiles() {
		if f.FileName() == name {
			file = f
			break
		}
	}
	if file == nil {
		return nil, errors.New("TY9101: upstream did not load the probe source")
	}
	s := &Session{program: p, file: file, text: source, scalar: scalar, snapshot: Snapshot{Protocol: Protocol, UpstreamCommit: UpstreamCommit, SourceSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(source))), Diagnostics: []Diagnostic{}}}
	ctx := context.Background()
	diagnostics := p.GetSyntacticDiagnostics(ctx, nil)
	diagnostics = append(diagnostics, p.GetGlobalDiagnostics(ctx)...)
	diagnostics = append(diagnostics, p.GetSemanticDiagnostics(ctx, nil)...)
	for _, d := range compiler.SortAndDeduplicateDiagnostics(diagnostics) {
		name, text := "", ""
		if d.File() != nil {
			name, text = d.File().FileName(), d.File().Text()
		}
		s.snapshot.Diagnostics = append(s.snapshot.Diagnostics, Diagnostic{Code: d.Code(), Message: d.String(), File: name, Start: position(text, d.Pos()), End: position(text, d.End())})
	}
	return s, nil
}

// Snapshot returns owned plain data. Mutating it cannot mutate the session.
func (s *Session) Snapshot() (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.program == nil {
		return Snapshot{}, ErrClosed
	}
	out := s.snapshot
	out.Diagnostics = append([]Diagnostic{}, s.snapshot.Diagnostics...)
	return out, nil
}

func (s *Session) TypeAt(byteOffset int) (TypeFact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.program == nil {
		return TypeFact{}, ErrClosed
	}
	if len(s.snapshot.Diagnostics) != 0 {
		return TypeFact{}, ErrSourceDiagnostics
	}
	if byteOffset < 0 || byteOffset >= len(s.text) || !utf8.RuneStart(s.text[byteOffset]) {
		return TypeFact{}, errors.New("TY1103: query must identify a UTF-8 character boundary inside the source")
	}
	var best *ast.Node
	var walk func(*ast.Node) bool
	walk = func(n *ast.Node) bool {
		if n == nil || byteOffset < n.Pos() || byteOffset >= n.End() {
			return false
		}
		best = n
		n.ForEachChild(walk)
		return false
	}
	walk(s.file.AsNode())
	if best == nil {
		return TypeFact{}, errors.New("TY1103: no source node at query")
	}
	start := scanner.GetTokenPosOfNode(best, s.file, false)
	if start > byteOffset {
		return TypeFact{}, errors.New("TY1103: query is in trivia, not a token")
	}
	c, release := s.program.GetCheckerPool().GetChecker(context.Background(), s.file)
	defer release()
	typ := c.GetTypeAtLocation(best)
	if typ == nil {
		return TypeFact{}, errors.New("TY1103: node has no queryable type")
	}
	return TypeFact{SourceSHA256: s.snapshot.SourceSHA256, NodeKind: best.Kind.String(), Text: s.text[start:best.End()], Display: c.TypeToString(typ), Start: position(s.text, start), End: position(s.text, best.End())}, nil
}

// Close invalidates further queries and releases our references. It does not
// force GC, free all upstream global caches, or invalidate prior value snapshots.
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.program, s.file, s.text = nil, nil, ""
	s.snapshot = Snapshot{}
}

func position(text string, offset int) Position {
	if offset < 0 {
		offset = 0
	}
	if offset > len(text) {
		offset = len(text)
	}
	line, column := 1, 1
	for i := 0; i < offset; {
		r, size := utf8.DecodeRuneInString(text[i:])
		if i+size > offset {
			break
		}
		i += size
		switch r {
		case '\r':
			if i < offset && strings.HasPrefix(text[i:], "\n") {
				i++
			}
			line++
			column = 1
		case '\n', '\u2028', '\u2029':
			line++
			column = 1
		default:
			column++
			if r > 0xffff {
				column++
			}
		}
	}
	return Position{ByteOffset: offset, Line: line, UTF16Column: column}
}
