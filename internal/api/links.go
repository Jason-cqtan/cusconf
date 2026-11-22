package api

import (
	"errors"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/cusconf/cusconf/internal/ruleset"
	"github.com/cusconf/cusconf/internal/store"
)

// --- short links ---

func (s *Server) serveShort(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	tok, err := s.st.TokenByShort(code)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	s.serveToken(w, r, tok.Token, s.publicBase(r)+"/s/"+code)
}

// createShort gives a token a short link code (or a new one, replacing the old).
func (s *Server) createShort(w http.ResponseWriter, r *http.Request) {
	tok, err := s.st.GetToken(chi.URLParam(r, "token"))
	if err != nil {
		storeErr(w, err)
		return
	}
	var code string
	for n := 6; code == ""; n++ {
		for try := 0; try < 5; try++ {
			c := store.NewShortCode(n)
			if _, err := s.st.TokenByShort(c); errors.Is(err, store.ErrNotFound) {
				code = c
				break
			}
		}
	}
	tok.Short = code
	if err := s.st.PutToken(tok); err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, tok)
}

func (s *Server) deleteShort(w http.ResponseWriter, r *http.Request) {
	tok, err := s.st.GetToken(chi.URLParam(r, "token"))
	if err != nil {
		storeErr(w, err)
		return
	}
	tok.Short = ""
	if err := s.st.PutToken(tok); err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, tok)
}

// --- rule set mirror ---

// mirrorFunc returns the render mirror hook for this request's public base URL.
func (s *Server) mirrorFunc(r *http.Request) func(upstream, behavior, format string) string {
	if s.mirror == nil {
		return nil
	}
	base := s.publicBase(r)
	return func(upstream, behavior, format string) string {
		return base + "/rs/" + s.mirror.Register(upstream, behavior) + "." + format
	}
}

// mirrorLocal reports whether mirror URLs for this request would point at the
// loopback address (no CUSCONF_BASE_URL, page opened as localhost).
func (s *Server) mirrorLocal(r *http.Request) bool {
	if s.baseURL != "" {
		return false
	}
	u, err := url.Parse(s.publicBase(r))
	if err != nil {
		return false
	}
	h := u.Hostname()
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return true
	}
	ip, err := netip.ParseAddr(h)
	return err == nil && ip.IsLoopback()
}

// serveRule serves a mirrored list: /rs/{id}.{format}.
func (s *Server) serveRule(w http.ResponseWriter, r *http.Request) {
	id, format, ok := strings.Cut(chi.URLParam(r, "file"), ".")
	if !ok || !ruleset.ValidFormat(format) || s.mirror == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	rc, err := s.mirror.Get(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "upstream unavailable: "+err.Error(), http.StatusBadGateway)
		return
	}
	body, ct := ruleset.Serve(rc, format)
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Last-Modified", time.Unix(rc.FetchedAt, 0).UTC().Format(http.TimeFormat))
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(body)
}

func (s *Server) listMirror(w http.ResponseWriter, _ *http.Request) {
	v, err := s.st.ListRules()
	if err != nil {
		storeErr(w, err)
		return
	}
	if v == nil {
		v = []store.RuleCache{}
	}
	writeJSON(w, v)
}

func (s *Server) refreshMirror(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.mirror.Refresh(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			storeErr(w, err)
			return
		}
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	rc, err := s.st.GetRule(id)
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, rc)
}

func (s *Server) deleteMirror(w http.ResponseWriter, r *http.Request) {
	if err := s.st.DeleteRule(chi.URLParam(r, "id")); err != nil {
		storeErr(w, err)
		return
	}
	s.mirror.Forget(chi.URLParam(r, "id"))
	w.WriteHeader(http.StatusNoContent)
}
