package web

import (
	"net/http"

	webassets "github.com/zhenghung/court-booking-bot/web"
)

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	// Serve index.html for / and static files otherwise. Auth already checked.
	if r.URL.Path == "/" {
		http.ServeFileFS(w, r, webassets.FS, "index.html")
		return
	}
	fileServer := http.FileServer(http.FS(webassets.FS))
	// Strip nothing; paths are /app.js, /style.css
	fileServer.ServeHTTP(w, r)
}
