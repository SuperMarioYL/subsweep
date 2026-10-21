// Package server runs the local web UI: a localhost-only HTTP server that
// parses dropped bill exports, keeps per-item confirmation state, and
// serves the renewal audit report. Data lives in memory only — nothing is
// persisted, nothing leaves the machine.
package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime/multipart"
	"net/http"
	"sync"
	"time"

	"github.com/SuperMarioYL/subsweep/internal/bills"
	"github.com/SuperMarioYL/subsweep/internal/catalog"
	"github.com/SuperMarioYL/subsweep/internal/match"
	"github.com/SuperMarioYL/subsweep/internal/report"
)

// DefaultAddr is the loopback address the UI listens on.
const DefaultAddr = "127.0.0.1:8787"

const maxUploadBytes = 32 << 20 // per uploaded bill file

// Options configures the local server.
type Options struct {
	Addr string           // default DefaultAddr
	KB   *catalog.Catalog // embedded service knowledge base
	Web  fs.FS            // embedded UI assets (index.html, app.js, style.css)
	LLM  match.LLMConfig  // optional merchant-classification assist, off by default
}

// Server holds one local session: parsed statements, the user's per-row
// confirmation verdicts, and any merchant-name overrides.
type Server struct {
	opts    Options
	matcher *match.Matcher

	mu         sync.Mutex
	llmCfg     match.LLMConfig
	llm        *match.LLM
	statements []*bills.Statement
	statuses   map[string]report.Status
	overrides  map[string]report.Service
}

// New creates a server session.
func New(opts Options) *Server {
	if opts.Addr == "" {
		opts.Addr = DefaultAddr
	}
	return &Server{
		opts:      opts,
		matcher:   match.New(opts.KB),
		llmCfg:    opts.LLM,
		llm:       match.NewLLM(opts.LLM),
		statuses:  map[string]report.Status{},
		overrides: map[string]report.Service{},
	}
}

// Handler assembles the UI routes. Pattern-method routes need Go 1.22+.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(s.opts.Web))
	mux.HandleFunc("POST /api/bills", s.handleUpload)
	mux.HandleFunc("GET /api/report", s.handleReport)
	mux.HandleFunc("POST /api/status", s.handleStatus)
	mux.HandleFunc("POST /api/pick", s.handlePick)
	mux.HandleFunc("POST /api/suggest", s.handleSuggest)
	mux.HandleFunc("POST /api/llm", s.handleLLMConfig)
	mux.HandleFunc("GET /api/export/report.md", s.handleExportMarkdown)
	mux.HandleFunc("POST /api/reset", s.handleReset)
	return mux
}

// Addr is the resolved listen address.
func (s *Server) Addr() string { return s.opts.Addr }

// uploadedFile is the per-file result of one drop.
type uploadedFile struct {
	File         string `json:"file"`
	Platform     string `json:"platform,omitempty"`
	Transactions int    `json:"transactions,omitempty"`
	Suspects     int    `json:"suspects,omitempty"`
	Error        string `json:"error,omitempty"`
}

// handleUpload parses one or more bill exports posted as multipart files.
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		httpError(w, http.StatusBadRequest, "读取上传内容失败: "+err.Error())
		return
	}
	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		httpError(w, http.StatusBadRequest, "未收到任何账单文件")
		return
	}

	results := make([]uploadedFile, 0, len(files))
	s.mu.Lock()
	for _, fh := range files {
		res := s.ingest(fh)
		results = append(results, res)
	}
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{"files": results})
}

// ingest parses one uploaded file into the session. Parse errors are
// reported per file so one bad export does not sink a mixed drop.
func (s *Server) ingest(fh *multipart.FileHeader) uploadedFile {
	res := uploadedFile{File: fh.Filename}
	f, err := fh.Open()
	if err != nil {
		res.Error = err.Error()
		return res
	}
	defer f.Close()

	raw, err := io.ReadAll(io.LimitReader(f, maxUploadBytes))
	if err != nil {
		res.Error = err.Error()
		return res
	}
	st, err := bills.Parse(raw)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	s.statements = append(s.statements, st)
	res.Platform = string(st.Platform)
	res.Transactions = len(st.Deductions)
	res.Suspects = len(st.Suspects())
	return res
}

// overrideCatalog layers user-picked services over the embedded knowledge
// base, implementing report.Catalog.
type overrideCatalog struct {
	base      report.Catalog
	overrides map[string]report.Service
}

func (o overrideCatalog) Find(merchant string) (report.Service, bool) {
	if svc, ok := o.overrides[merchant]; ok {
		return svc, true
	}
	return o.base.Find(merchant)
}

// snapshot builds the current report from the session state.
func (s *Server) snapshot() *report.Report {
	suspects := make([]bills.Suspect, 0, 16)
	for _, st := range s.statements {
		suspects = append(suspects, st.Suspects()...)
	}
	catalogue := overrideCatalog{
		base:      match.Catalog{KB: s.opts.KB},
		overrides: s.overrides,
	}
	return report.Build(report.Input{
		Statements: s.statements,
		Suspects:   suspects,
		Catalog:    catalogue,
		Statuses:   s.statuses,
		Now:        time.Now(),
	})
}

func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	rep := s.snapshot()
	llmOn := s.llm.Enabled()
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"report":      rep,
		"llm_enabled": llmOn,
	})
}

// handleStatus applies the user's verdict to every row of one signing.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RowIDs []string `json:"row_ids"`
		Status string   `json:"status"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		httpError(w, http.StatusBadRequest, "请求格式错误: "+err.Error())
		return
	}
	switch body.Status {
	case "confirmed", "unrecognized", "pending":
	default:
		httpError(w, http.StatusBadRequest, fmt.Sprintf("未知状态 %q", body.Status))
		return
	}

	s.mu.Lock()
	for _, id := range body.RowIDs {
		if id == "" {
			continue
		}
		if body.Status == "pending" {
			delete(s.statuses, id)
			continue
		}
		s.statuses[id] = report.Status(body.Status)
	}
	rep := s.snapshot()
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{"report": rep})
}

// handlePick lets the user correct a mis-matched service for one merchant.
func (s *Server) handlePick(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Merchant      string `json:"merchant"`
		CanonicalName string `json:"canonical_name"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		httpError(w, http.StatusBadRequest, "请求格式错误: "+err.Error())
		return
	}

	s.mu.Lock()
	if svc, ok := s.opts.KB.Find(body.CanonicalName); ok {
		s.overrides[body.Merchant] = toReport(svc)
	} else {
		delete(s.overrides, body.Merchant)
	}
	rep := s.snapshot()
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{"report": rep})
}

func toReport(svc catalog.Service) report.Service {
	return report.Service{
		CanonicalName: svc.CanonicalName,
		Cancel: report.CancelPath{
			App:   svc.Cancel.App,
			Steps: svc.Cancel.Steps,
			URL:   svc.Cancel.URL,
		},
	}
}

// handleSuggest ranks catalog candidates for a raw merchant name; when the
// local rules find nothing and the LLM assist is on, the merchant name (and
// nothing else) is offered to the model.
func (s *Server) handleSuggest(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Merchant string `json:"merchant"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		httpError(w, http.StatusBadRequest, "请求格式错误: "+err.Error())
		return
	}

	type suggestion struct {
		CanonicalName string  `json:"canonical_name"`
		Score         float64 `json:"score"`
		Source        string  `json:"source"`
	}
	suggestions := []suggestion{}
	for _, c := range s.matcher.Suggest(body.Merchant, 5) {
		suggestions = append(suggestions, suggestion{
			CanonicalName: c.Service.CanonicalName,
			Score:         c.Score,
			Source:        "catalog",
		})
	}

	var llmGuess, llmError string
	s.mu.Lock()
	llm, kb := s.llm, s.opts.KB
	s.mu.Unlock()
	if len(suggestions) == 0 && llm.Enabled() && body.Merchant != "" {
		names := make([]string, 0, kb.Size())
		for _, svc := range kb.Services() {
			names = append(names, svc.CanonicalName)
		}
		guess, err := llm.Classify(r.Context(), body.Merchant, names)
		if err != nil && !errors.Is(err, match.ErrDisabled) {
			llmError = err.Error()
		}
		if guess != "" {
			llmGuess = guess
			suggestions = append(suggestions, suggestion{
				CanonicalName: guess,
				Score:         0,
				Source:        "llm",
			})
		}
	}

	s.mu.Lock()
	llmOn, llmModel := s.llm.Enabled(), s.llm.Model()
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"merchant":    body.Merchant,
		"suggestions": suggestions,
		"llm": map[string]any{
			"enabled": llmOn,
			"model":   llmModel,
			"guess":   llmGuess,
			"error":   llmError,
		},
	})
}

// handleLLMConfig stores the user's own API key for this session. It lives
// in memory only and is never written to disk.
func (s *Server) handleLLMConfig(w http.ResponseWriter, r *http.Request) {
	var body struct {
		APIKey string `json:"api_key"`
		Model  string `json:"model"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		httpError(w, http.StatusBadRequest, "请求格式错误: "+err.Error())
		return
	}

	s.mu.Lock()
	s.llmCfg.APIKey = body.APIKey
	s.llmCfg.Enabled = body.APIKey != ""
	if body.Model != "" {
		s.llmCfg.Model = body.Model
	}
	s.llm = match.NewLLM(s.llmCfg)
	model := s.llm.Model()
	enabled := s.llm.Enabled()
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{"enabled": enabled, "model": model})
}

// handleExportMarkdown serves the shareable Markdown report as a download.
func (s *Server) handleExportMarkdown(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	rep := s.snapshot()
	s.mu.Unlock()

	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="subsweep-report.md"`)
	w.Write([]byte(rep.Markdown()))
}

// handleReset clears the in-memory session.
func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.statements = nil
	s.statuses = map[string]report.Status{}
	s.overrides = map[string]report.Service{}
	rep := s.snapshot()
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"report": rep})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// The response header is already committed; nothing else to do.
		return
	}
}

func httpError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": msg})
}
