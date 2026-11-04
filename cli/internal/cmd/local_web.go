package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

// This launcher only supplies already-built application resources. All local
// pairing, identity keys, encrypted data and receipts stay in the browser.
// It never proxies a cloud API or exposes the user's filesystem to LAN peers.
func newLocalWebCommand() *cobra.Command {
	var directory, listen, ready string
	var open bool
	c := &cobra.Command{Use: "local-web", Short: "open the bundled Web app without an internet service", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		host, _, err := net.SplitHostPort(listen)
		if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
			return fmt.Errorf("local Web app must listen on a loopback IP")
		}
		listener, err := net.Listen("tcp", listen)
		if err != nil {
			return err
		}
		defer listener.Close()
		handler, err := localWebHandler(directory, listener.Addr().String())
		if err != nil {
			return err
		}
		server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		address := "http://" + listener.Addr().String()
		fmt.Fprintln(cmd.OutOrStdout(), "Pinelink: "+address)
		if ready != "" {
			data, _ := json.Marshal(map[string]string{"url": address, "version": BuildVersion})
			if err = os.WriteFile(ready, data, 0600); err != nil {
				return err
			}
		}
		if open {
			go func() {
				var c *exec.Cmd
				switch runtime.GOOS {
				case "windows":
					c = exec.Command("rundll32", "url.dll,FileProtocolHandler", address)
				case "darwin":
					c = exec.Command("open", address)
				default:
					c = exec.Command("xdg-open", address)
				}
				_ = c.Run()
			}()
		}
		result := make(chan error, 1)
		go func() { result <- server.Serve(listener) }()
		select {
		case err = <-result:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			return server.Shutdown(shutdown)
		}
	}}
	c.Flags().StringVar(&directory, "web-dir", "web", "directory containing the matching Web release")
	c.Flags().StringVar(&listen, "listen", "127.0.0.1:17643", "loopback address for the local application")
	c.Flags().StringVar(&ready, "ready-file", "", "write local readiness metadata for automation")
	c.Flags().BoolVar(&open, "open", true, "open the application in the default browser")
	return c
}

func localWebHandler(directory, address string) (http.Handler, error) {
	root, err := filepath.Abs(directory)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(filepath.Join(root, "index.html"))
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("Web bundle is missing index.html")
	}
	files := map[string]string{}
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		files["/"+filepath.ToSlash(rel)] = p
		return nil
	})
	if err != nil {
		return nil, err
	}
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	hosts := map[string]bool{address: true, net.JoinHostPort("localhost", port): true}
	pages := map[string]bool{"/": true, "/files": true, "/notifications": true, "/connect": true, "/tasks": true, "/history": true, "/local-files": true, "/settings": true}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'wasm-unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self'; media-src 'self' blob:; connect-src 'self' blob:; worker-src 'self' blob:; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		w.Header().Set("Cache-Control", "no-store")
		if !hosts[strings.ToLower(r.Host)] || r.Header.Get("Origin") != "" && r.Header.Get("Origin") != "http://"+r.Host || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path == "/api/v1/auth/capabilities" {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == http.MethodGet {
				_ = json.NewEncoder(w).Encode(map[string]any{"enabled": false, "required": false, "guests": true, "local_only": true, "guest_max_file_bytes": 64 << 20})
			}
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") || path.Clean(r.URL.Path) != r.URL.Path {
			http.NotFound(w, r)
			return
		}
		file := files[r.URL.Path]
		if pages[r.URL.Path] {
			file = files["/index.html"]
		}
		if file == "" {
			http.NotFound(w, r)
			return
		}
		// Recheck the chosen path in case a local process changed it after startup.
		resolved, err := filepath.EvalSymlinks(file)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		rel, err := filepath.Rel(root, resolved)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			http.NotFound(w, r)
			return
		}
		if content := mime.TypeByExtension(filepath.Ext(file)); content != "" {
			w.Header().Set("Content-Type", content)
		}
		http.ServeFile(w, r, file)
	}), nil
}
