// Package api exposes the admin REST API, the subscription endpoint and the web UI.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/cusconf/cusconf/internal/capability"
	"github.com/cusconf/cusconf/internal/engine"
	"github.com/cusconf/cusconf/internal/node"
	"github.com/cusconf/cusconf/internal/parser"
	"github.com/cusconf/cusconf/internal/profile"
	"github.com/cusconf/cusconf/internal/render"
	"github.com/cusconf/cusconf/internal/ruleset"
	"github.com/cusconf/cusconf/internal/source"
	"github.com/cusconf/cusconf/internal/store"
	"github.com/cusconf/cusconf/internal/tester"
)

type Server struct {
	st      *store.Store
	src     *source.Manager
	tests   *tester.Manager
	mirror  *ruleset.Mirror
	auth    *auth
	baseURL string // public base URL, e.g. https://conf.example.com (optional)
	web     fs.FS
}

func New(st *store.Store, src *source.Manager, tests *tester.Manager, mirror *ruleset.Mirror, web fs.FS) (*Server, error) {
	a, err := loadAuth(st)
	if err != nil {
		return nil, err
	}
	return &Server{st: st, src: src, tests: tests, mirror: mirror, auth: a, baseURL: strings.TrimRight(os.Getenv("CUSCONF_BASE_URL"), "/"), web: web}, nil
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RealIP, middleware.Recoverer)

	r.Get("/sub/{token}", s.serveSub)
	r.Get("/s/{code}", s.serveShort)
	r.Get("/rs/{file}", s.serveRule)
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })

	r.Route("/api", func(r chi.Router) {
		r.Use(middleware.Logger)
		r.Post("/login", s.login)
		r.Post("/sync/results", s.syncImport) // authenticated by the sync key
		r.Group(func(r chi.Router) {
			r.Use(s.auth.middleware)
			r.Get("/capabilities", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, capability.Default) })
			r.Get("/regions", s.regions)

			r.Get("/sources", s.listSources)
			r.Post("/sources", s.saveSource)
			r.Put("/sources/{id}", s.saveSource)
			r.Delete("/sources/{id}", s.deleteSource)
			r.Post("/sources/{id}/refresh", s.refreshSource)
			r.Get("/sources/{id}/nodes", s.sourceNodes)
			r.Post("/parse", s.parse)
			r.Get("/nodes", s.allNodes)

			r.Get("/tests/settings", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, s.tests.Settings()) })
			r.Put("/tests/settings", s.saveTestSettings)
			r.Get("/tests/services", s.testServices)
			r.Get("/tests/results", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, s.tests.Results()) })
			r.Get("/tests/job", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, s.tests.Status()) })
			r.Post("/tests/job", s.startTest)
			r.Post("/tests/node", s.testNode)
			r.Get("/sync/key", s.syncKey)
			r.Post("/sync/key", s.syncKey)
			r.Post("/sync/push", s.syncPush)
			r.Delete("/tests/job", func(w http.ResponseWriter, _ *http.Request) { s.tests.Stop(); writeJSON(w, s.tests.Status()) })

			r.Get("/profiles", s.listProfiles)
			r.Get("/profiles/template", s.template)
			r.Get("/profiles/{id}", s.getProfile)
			r.Post("/profiles", s.saveProfile)
			r.Put("/profiles/{id}", s.saveProfile)
			r.Delete("/profiles/{id}", s.deleteProfile)
			r.Post("/preview", s.preview)
			r.Post("/resolve", s.resolve)

			r.Get("/tokens", s.listTokens)
			r.Post("/tokens", s.saveToken)
			r.Put("/tokens/{token}", s.saveToken)
			r.Delete("/tokens/{token}", s.deleteToken)
			r.Get("/tokens/{token}/access", s.tokenAccess)
			r.Post("/tokens/{token}/short", s.createShort)
			r.Delete("/tokens/{token}/short", s.deleteShort)

			r.Get("/mirror", s.listMirror)
			r.Post("/mirror/{id}/refresh", s.refreshMirror)
			r.Delete("/mirror/{id}", s.deleteMirror)
		})
	})

	if s.web != nil {
		r.Handle("/*", spa(s.web))
	}
	return r
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func storeErr(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	writeErr(w, http.StatusInternalServerError, err.Error())
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 10<<20)).Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return false
	}
	return true
}

func (s *Server) publicBase(r *http.Request) string {
	if s.baseURL != "" {
		return s.baseURL
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	return scheme + "://" + host
}

// --- auth ---

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body struct{ Password string }
	if !decode(w, r, &body) {
		return
	}
	if !s.auth.check(body.Password) {
		time.Sleep(time.Second) // slow down brute force
		writeErr(w, http.StatusUnauthorized, "wrong password")
		return
	}
	tok, exp := s.auth.issue()
	writeJSON(w, map[string]any{"token": tok, "expiresAt": exp})
}

// --- sources & nodes ---

type nodeView struct {
	*node.Node
	Key    string `json:"key"`
	Region string `json:"region"`
	Info   bool   `json:"info,omitempty"` // airport traffic/expiry pseudo-node
}

func views(ns []*node.Node) []nodeView {
	out := make([]nodeView, len(ns))
	for i, n := range ns {
		out[i] = nodeView{Node: n, Key: n.Key(), Region: node.Region(n), Info: node.IsInfo(n)}
	}
	return out
}

func (s *Server) regions(w http.ResponseWriter, _ *http.Request) {
	type reg struct{ Code, Name string }
	var out []reg
	for _, r := range node.Regions {
		out = append(out, reg{r.Code, r.Name})
	}
	writeJSON(w, out)
}

func (s *Server) listSources(w http.ResponseWriter, _ *http.Request) {
	v, err := s.st.ListSources()
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, v)
}

func (s *Server) saveSource(w http.ResponseWriter, r *http.Request) {
	var in store.Source
	if !decode(w, r, &in) {
		return
	}
	if id := chi.URLParam(r, "id"); id != "" {
		old, err := s.st.GetSource(id)
		if err != nil {
			storeErr(w, err)
			return
		}
		in.ID = id
		// keep status fields
		in.FetchedAt, in.NodeCount, in.Info, in.Format, in.Warnings, in.LastError = old.FetchedAt, old.NodeCount, old.Info, old.Format, old.Warnings, old.LastError
	} else {
		in.ID = store.NewID()
	}
	switch in.Kind {
	case "url":
		if u, err := url.Parse(in.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			writeErr(w, http.StatusBadRequest, "invalid subscription URL")
			return
		}
	case "manual":
	default:
		writeErr(w, http.StatusBadRequest, "kind must be url or manual")
		return
	}
	if in.Name == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	if err := s.st.PutSource(&in); err != nil {
		storeErr(w, err)
		return
	}
	// Populate the node cache right away; report fetch errors in the status, not as a failure.
	if err := s.src.Refresh(r.Context(), &in); err != nil {
		slog.Warn("initial refresh", "source", in.Name, "err", err)
	}
	writeJSON(w, in)
}

func (s *Server) deleteSource(w http.ResponseWriter, r *http.Request) {
	if err := s.st.DeleteSource(chi.URLParam(r, "id")); err != nil {
		storeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) refreshSource(w http.ResponseWriter, r *http.Request) {
	src, err := s.st.GetSource(chi.URLParam(r, "id"))
	if err != nil {
		storeErr(w, err)
		return
	}
	if err := s.src.Refresh(r.Context(), src); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, src)
}

func (s *Server) sourceNodes(w http.ResponseWriter, r *http.Request) {
	ns, err := s.st.GetNodes(chi.URLParam(r, "id"))
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, views(ns))
}

func (s *Server) allNodes(w http.ResponseWriter, _ *http.Request) {
	ns, err := s.nodes()
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, views(ns))
}

func (s *Server) parse(w http.ResponseWriter, r *http.Request) {
	var body struct{ Content string }
	if !decode(w, r, &body) {
		return
	}
	res := parser.Parse([]byte(body.Content))
	errs := make([]string, len(res.Errors))
	for i, e := range res.Errors {
		errs[i] = e.Error()
	}
	writeJSON(w, map[string]any{"format": res.Format, "nodes": views(res.Nodes), "errors": errs})
}

// --- profiles ---

func (s *Server) listProfiles(w http.ResponseWriter, _ *http.Request) {
	v, err := s.st.ListProfiles()
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, v)
}

func (s *Server) getProfile(w http.ResponseWriter, r *http.Request) {
	p, err := s.st.GetProfile(chi.URLParam(r, "id"))
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, p)
}

func (s *Server) saveProfile(w http.ResponseWriter, r *http.Request) {
	var p profile.Profile
	if !decode(w, r, &p) {
		return
	}
	if id := chi.URLParam(r, "id"); id != "" {
		if _, err := s.st.GetProfile(id); err != nil {
			storeErr(w, err)
			return
		}
		p.ID = id
	} else {
		p.ID = store.NewID()
	}
	if p.Name == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	if err := p.Validate(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.st.PutProfile(&p); err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, p)
}

func (s *Server) deleteProfile(w http.ResponseWriter, r *http.Request) {
	if err := s.st.DeleteProfile(chi.URLParam(r, "id")); err != nil {
		storeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// preview renders an (unsaved) profile for a target.
func (s *Server) preview(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Profile profile.Profile   `json:"profile"`
		Target  capability.Target `json:"target"`
	}
	if !decode(w, r, &body) {
		return
	}
	nodes, err := s.nodes()
	if err != nil {
		storeErr(w, err)
		return
	}
	res, err := engine.Generate(engine.Request{Profile: &body.Profile, Nodes: nodes, Target: body.Target,
		SubscriptionURLs: s.subscriptionURLs(), Mirror: s.mirrorFunc(r), MirrorLocal: s.mirrorLocal(r)})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, res)
}

// resolve expands an (unsaved) profile's groups to concrete members, so the
// editor can show which nodes each filter matches.
func (s *Server) resolve(w http.ResponseWriter, r *http.Request) {
	var p profile.Profile
	if !decode(w, r, &p) {
		return
	}
	if err := p.Validate(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	all, err := s.nodes()
	if err != nil {
		storeErr(w, err)
		return
	}
	var warns render.Warnings
	nodes := engine.SelectNodes(&p, all)
	groups := engine.ResolveGroups(&p, nodes, &warns)
	members := map[string][]string{}
	for _, g := range groups {
		members[g.Name] = g.Members
	}
	// scopeNodes skips the global filter: the editor offers its choices from them, so
	// picking one option there does not hide the others.
	unfiltered := p
	unfiltered.Filter = profile.Filter{}
	scope := engine.SelectNodes(&unfiltered, all)
	writeJSON(w, map[string]any{"nodes": views(nodes), "scopeNodes": views(scope), "members": members, "warnings": warns})
}

// --- tokens ---

func (s *Server) listTokens(w http.ResponseWriter, _ *http.Request) {
	v, err := s.st.ListTokens()
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, v)
}

func (s *Server) saveToken(w http.ResponseWriter, r *http.Request) {
	var in store.Token
	if !decode(w, r, &in) {
		return
	}
	if t := chi.URLParam(r, "token"); t != "" {
		old, err := s.st.GetToken(t)
		if err != nil {
			storeErr(w, err)
			return
		}
		in.Token, in.CreatedAt, in.Uses, in.LastUsedAt, in.LastIP, in.LastUA = t, old.CreatedAt, old.Uses, old.LastUsedAt, old.LastIP, old.LastUA
		in.Short = old.Short
	} else {
		in.Token, in.CreatedAt = store.NewToken(), time.Now().Unix()
	}
	if _, err := s.st.GetProfile(in.ProfileID); err != nil {
		writeErr(w, http.StatusBadRequest, "unknown profile")
		return
	}
	norm := capability.Target{Client: in.Target.Client, Platform: in.Target.Platform, Version: in.Target.Version}
	if err := capability.Default.Validate(&norm); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	in.Target.Platform = norm.Platform // version stays as given: empty follows the latest
	if in.Name == "" {
		in.Name = fmt.Sprintf("%s-%s", in.Target.Client, in.Target.Platform)
	}
	if err := s.st.PutToken(&in); err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, in)
}

func (s *Server) deleteToken(w http.ResponseWriter, r *http.Request) {
	if err := s.st.DeleteToken(chi.URLParam(r, "token")); err != nil {
		storeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) tokenAccess(w http.ResponseWriter, r *http.Request) {
	v, err := s.st.ListAccess(chi.URLParam(r, "token"), 100)
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, v)
}

// --- subscription output ---

func (s *Server) serveSub(w http.ResponseWriter, r *http.Request) {
	tokID := chi.URLParam(r, "token")
	s.serveToken(w, r, tokID, s.publicBase(r)+"/sub/"+tokID)
}

// serveToken writes the config of a subscription token; selfURL is the URL the
// client used (the managed-config / update URL written into the config).
func (s *Server) serveToken(w http.ResponseWriter, r *http.Request, tokID, selfURL string) {
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	if ip == "" {
		ip = r.RemoteAddr
	}
	status := http.StatusOK
	defer func() {
		s.st.LogAccess(tokID, store.Access{At: time.Now().Unix(), IP: ip, UserAgent: r.UserAgent(), Status: status})
	}()
	fail := func(code int, msg string) {
		status = code
		http.Error(w, msg, code)
	}

	tok, err := s.st.GetToken(tokID)
	if err != nil {
		fail(http.StatusNotFound, "not found")
		return
	}
	if err := tok.Usable(time.Now()); err != nil {
		fail(http.StatusGone, "subscription "+err.Error())
		return
	}
	p, err := s.st.GetProfile(tok.ProfileID)
	if err != nil {
		fail(http.StatusNotFound, "profile not found")
		return
	}
	nodes, err := s.nodes()
	if err != nil {
		fail(http.StatusInternalServerError, "internal error")
		return
	}
	res, err := engine.Generate(engine.Request{
		Profile: p, Nodes: nodes, Target: tok.Target,
		ManagedURL: selfURL, SubscriptionURLs: s.subscriptionURLs(), Mirror: s.mirrorFunc(r), MirrorLocal: s.mirrorLocal(r),
	})
	if err != nil {
		fail(http.StatusInternalServerError, err.Error())
		return
	}

	tok.Uses++
	tok.LastUsedAt, tok.LastIP, tok.LastUA = time.Now().Unix(), ip, r.UserAgent()
	s.st.PutToken(tok)

	ext := capability.Default.Clients[tok.Target.Client].Ext
	iv := p.General.UpdateInterval
	if iv <= 0 {
		iv = 86400
	}
	w.Header().Set("Content-Type", res.ContentType)
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(p.Name+ext))
	w.Header().Set("Profile-Update-Interval", fmt.Sprint(max(iv/3600, 1)))
	w.Header().Set("Cache-Control", "no-store")
	if info := s.subInfo(p); info != "" {
		w.Header().Set("Subscription-Userinfo", info)
	}
	w.Write([]byte(res.Content))
}

// subInfo aggregates traffic/expiry of the profile's URL sources.
func (s *Server) subInfo(p *profile.Profile) string {
	srcs, err := s.st.ListSources()
	if err != nil {
		return ""
	}
	var up, down, total, expire int64
	found := false
	for _, src := range srcs {
		if src.Info == nil || !src.Enabled || (len(p.Sources) > 0 && !contains(p.Sources, src.ID)) {
			continue
		}
		found = true
		up, down, total = up+src.Info.Upload, down+src.Info.Download, total+src.Info.Total
		if src.Info.Expire > 0 && (expire == 0 || src.Info.Expire < expire) {
			expire = src.Info.Expire
		}
	}
	if !found {
		return ""
	}
	return fmt.Sprintf("upload=%d; download=%d; total=%d; expire=%d", up, down, total, expire)
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// spa serves static files, falling back to index.html for client routes.
func spa(root fs.FS) http.Handler {
	files := http.FileServer(http.FS(root))
	if _, err := fs.Stat(root, "index.html"); err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "web UI not built: run `npm run build` in web/ and rebuild", http.StatusNotFound)
		})
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p != "" {
			if _, err := fs.Stat(root, p); err == nil {
				if strings.HasPrefix(p, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		files.ServeHTTP(w, r2)
	})
}

// nodes returns all nodes of enabled sources with their latest test summary attached.
func (s *Server) nodes() ([]*node.Node, error) {
	ns, err := s.st.AllNodes()
	if err != nil {
		return nil, err
	}
	s.tests.Attach(ns)
	return ns, nil
}

func (s *Server) saveTestSettings(w http.ResponseWriter, r *http.Request) {
	var in tester.Settings
	if !decode(w, r, &in) {
		return
	}
	for i, src := range in.IPSources {
		if src.Name == "" || src.URL == "" {
			writeErr(w, http.StatusBadRequest, fmt.Sprintf("IP 源 %d：名称和地址必填", i+1))
			return
		}
	}
	if err := s.tests.SaveSettings(in); err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, s.tests.Settings())
}

func (s *Server) testServices(w http.ResponseWriter, _ *http.Request) {
	type svc struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	out := make([]svc, len(tester.Services))
	for i, x := range tester.Services {
		out[i] = svc{x.ID, x.Name}
	}
	writeJSON(w, map[string]any{"services": out, "defaultIpSources": tester.DefaultIPSources()})
}

// startTest tests the nodes with the given keys (all nodes when empty).
func (s *Server) startTest(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Keys     []string `json:"keys"`
		IP       bool     `json:"ip"`
		Services bool     `json:"services"`
	}
	if !decode(w, r, &in) {
		return
	}
	all, err := s.st.AllNodes()
	if err != nil {
		storeErr(w, err)
		return
	}
	nodes := all
	if len(in.Keys) > 0 {
		want := map[string]bool{}
		for _, k := range in.Keys {
			want[k] = true
		}
		nodes = nodes[:0:0]
		for _, n := range all {
			if want[n.Key()] {
				nodes = append(nodes, n)
			}
		}
	}
	if len(nodes) == 0 {
		writeErr(w, http.StatusBadRequest, "没有可测试的节点")
		return
	}
	job, err := s.tests.Start(nodes, tester.Checks{IP: in.IP, Services: in.Services}, false)
	if err != nil {
		code := http.StatusBadRequest
		if errors.Is(err, tester.ErrBusy) {
			code = http.StatusConflict
		}
		writeErr(w, code, err.Error())
		return
	}
	writeJSON(w, job)
}

// template builds the starter profile, shaping per-country groups from the current nodes.
func (s *Server) template(w http.ResponseWriter, _ *http.Request) {
	all, err := s.st.AllNodes()
	if err != nil {
		storeErr(w, err)
		return
	}
	counts, selfHosted := map[string]int{}, 0
	for _, n := range all {
		switch {
		case node.IsInfo(n):
		case n.SelfHosted:
			selfHosted++
		default:
			counts[node.Region(n)]++
		}
	}
	writeJSON(w, profile.Default(counts, selfHosted))
}

// testNode tests one node synchronously and returns its result.
func (s *Server) testNode(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Key      string `json:"key"`
		IP       bool   `json:"ip"`
		Services bool   `json:"services"`
	}
	if !decode(w, r, &in) {
		return
	}
	all, err := s.st.AllNodes()
	if err != nil {
		storeErr(w, err)
		return
	}
	var target *node.Node
	for _, n := range all {
		if n.Key() == in.Key && !node.IsInfo(n) {
			target = n
			break
		}
	}
	if target == nil {
		writeErr(w, http.StatusNotFound, "节点不存在，或是机场信息节点")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	writeJSON(w, s.tests.TestOne(ctx, target, tester.Checks{IP: in.IP, Services: in.Services}))
}

// subscriptionURLs lists the URLs of all enabled URL sources. Every one gets a
// DIRECT rule, not only the profile's: the device may refresh the others too.
func (s *Server) subscriptionURLs() []string {
	srcs, err := s.st.ListSources()
	if err != nil {
		return nil
	}
	var out []string
	for _, src := range srcs {
		if src.Kind == "url" && src.Enabled {
			out = append(out, src.URL)
		}
	}
	return out
}
