package api

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/cusconf/cusconf/internal/tester"
)

// syncImport receives test results pushed by another cusconf. It is not behind the
// admin login; the caller authenticates with this instance's sync key instead.
func (s *Server) syncImport(w http.ResponseWriter, r *http.Request) {
	key, err := s.tests.SyncKey()
	if err != nil {
		storeErr(w, err)
		return
	}
	got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(key)) != 1 {
		writeErr(w, http.StatusUnauthorized, tester.ErrSyncKey.Error())
		return
	}
	var in struct {
		Results []tester.Result     `json:"results"`
		Sources []tester.SyncSource `json:"sources"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<20)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad body: "+err.Error())
		return
	}
	nsrc, err := s.tests.ImportSources(in.Sources)
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, map[string]int{"imported": s.tests.Import(in.Results), "sources": nsrc})
}

func (s *Server) syncKey(w http.ResponseWriter, r *http.Request) {
	var key string
	var err error
	if r.Method == http.MethodPost {
		key, err = s.tests.RegenerateSyncKey()
	} else {
		key, err = s.tests.SyncKey()
	}
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, map[string]string{"key": key})
}

func (s *Server) syncPush(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.tests.Push(r.Context()))
}
