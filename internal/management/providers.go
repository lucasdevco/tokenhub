package management

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"

	"github.com/lucasdevco/tokenhub/internal/service"
)

func (s *Server) providers(w http.ResponseWriter, r *http.Request) {
	// This endpoint includes credentials, including on reads.
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Tokenhub-Session")), []byte(s.session)) != 1 {
		fail(w, http.StatusForbidden, fmt.Errorf("Invalid management session"))
		return
	}
	var result service.ProviderCatalog
	var err error
	if r.Method == http.MethodPost {
		var input service.ProviderChange
		if !decode(w, r, &input) {
			return
		}
		s.mu.Lock()
		result, err = s.Manager.ChangeProvider(input)
		s.mu.Unlock()
	} else {
		result, err = s.Manager.ReadProviders()
	}
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, service.ErrProviderConflict) {
			status = http.StatusConflict
		}
		fail(w, status, err)
		return
	}
	send(w, result)
}
