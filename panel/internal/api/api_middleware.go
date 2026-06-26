package api

import (
	"KorisPanel/panel/internal/auth"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := s.currentAdmin(r); !ok {
			writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

// requireFullAdmin blocks resellers — only owner/admin roles may proceed.
func (s *Server) requireFullAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, role, ok := s.currentAdmin(r)
		if !ok {
			writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
			return
		}
		if role == "reseller" {
			writeJSONCode(w, http.StatusForbidden, map[string]any{"ok": false, "error": "forbidden"})
			return
		}
		next(w, r)
	}
}

// RequireAdmin is the exported version of requireAdmin for use by the main package.
func (s *Server) RequireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return s.requireAdmin(next)
}

func (s *Server) requireCustomer(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.currentCustomer(r); !ok {
			writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

func (s *Server) currentAdmin(r *http.Request) (string, string, bool) {
	username, ok := auth.ReadSession(r, auth.AdminCookieName, s.Config.SessionSecret)
	if !ok {
		return "", "", false
	}
	var role string
	var active bool
	err := s.DB.QueryRow(`SELECT role,is_active FROM admins WHERE username=$1 LIMIT 1`, username).Scan(&role, &active)
	if err != nil || !active {
		return "", "", false
	}
	return username, role, true
}

func (s *Server) currentCustomer(r *http.Request) (string, bool) {
	username, ok := auth.ReadSession(r, auth.CustomerCookieName, s.Config.SessionSecret)
	if !ok {
		return "", false
	}
	var status string
	err := s.DB.QueryRow(`SELECT status FROM customers WHERE username=$1 AND deleted_at IS NULL LIMIT 1`, username).Scan(&status)
	if err == nil {
		return username, status != "disabled" && status != "deleted"
	}
	var count int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM radcheck WHERE username=$1`, username).Scan(&count); err != nil {
		return "", false
	}
	return username, count > 0
}

func (s *Server) count(query string, args ...any) int64 {
	var v int64
	_ = s.DB.QueryRow(query, args...).Scan(&v)
	return v
}

func (s *Server) sum(query string, args ...any) float64 {
	var v float64
	_ = s.DB.QueryRow(query, args...).Scan(&v)
	return v
}

func redirectTo(target string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target, http.StatusFound)
	}
}

func spaHandler(dir, prefix string, embedded fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}

		// Determine which filesystem to use: embedded (in binary) or disk (dev mode override)
		var serve func(name string) (fs.File, error)
		useEmbed := embedded != nil

		// If a disk directory is configured and exists, prefer it (allows hot-reload in dev)
		if dir != "" {
			if _, err := os.Stat(filepath.Join(dir, "index.html")); err == nil {
				useEmbed = false
			}
		}

		if useEmbed {
			serve = embedded.Open
		} else {
			if dir == "" {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`<html><body style="font-family:system-ui;background:#080a10;color:#f8fafc;padding:40px"><h1>Koris UI is not built yet</h1><p>Build the Vue app and rebuild the Go binary.</p></body></html>`))
				return
			}
			serve = func(name string) (fs.File, error) {
				return os.Open(filepath.Join(dir, filepath.FromSlash(name)))
			}
		}

		// Check index.html exists
		idx, err := serve("index.html")
		if err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`<html><body style="font-family:system-ui;background:#080a10;color:#f8fafc;padding:40px"><h1>Koris UI is not built yet</h1><p>Build the Vue app and rebuild the Go binary.</p></body></html>`))
			return
		}
		idx.Close()

		rel := strings.TrimPrefix(r.URL.Path, prefix)
		clean := path.Clean("/" + rel)
		if clean != "/" {
			assetPath := strings.TrimPrefix(clean, "/")
			if useEmbed {
				if f, err := embedded.Open(assetPath); err == nil {
					defer f.Close()
					stat, _ := f.Stat()
					if stat != nil && !stat.IsDir() {
						if strings.HasPrefix(assetPath, "assets/") {
							w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
						}
						http.ServeContent(w, r, stat.Name(), stat.ModTime(), f.(io.ReadSeeker))
						return
					}
				}
			} else {
				fullPath := filepath.Join(dir, filepath.FromSlash(assetPath))
				if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
					if strings.HasPrefix(assetPath, "assets/") {
						w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
					}
					http.ServeFile(w, r, fullPath)
					return
				}
			}
		}

		// SPA fallback: serve index.html
		w.Header().Set("Cache-Control", "no-store")
		if useEmbed {
			f, _ := embedded.Open("index.html")
			if f != nil {
				defer f.Close()
				stat, _ := f.Stat()
				http.ServeContent(w, r, "index.html", stat.ModTime(), f.(io.ReadSeeker))
			}
		} else {
			http.ServeFile(w, r, filepath.Join(dir, "index.html"))
		}
	})
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
