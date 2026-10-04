package gateway

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmaister/taronja-gateway/config"
)

// createStaticHandlerFunc generates the core handler function for static routes (without auth).
// This function continues to serve user-defined static routes from the OS filesystem.
func (g *Gateway) createStaticHandlerFunc(routeConfig config.RouteConfig) http.HandlerFunc {
	var fsPath string
	var isDir bool

	log.Printf("Static Route [%s]: Creating handler for route '%s'", routeConfig.Name, routeConfig.From)

	// Determine path from configuration
	if routeConfig.ToFile != "" {
		// Use ToFile directly as an independent path
		fsPath = routeConfig.ToFile
		log.Printf("Static Route [%s]: Using ToFile path: %s", routeConfig.Name, fsPath)
	} else if routeConfig.ToFolder != "" {
		// Use ToFolder directly
		fsPath = routeConfig.ToFolder
		log.Printf("Static Route [%s]: Using ToFolder path: %s", routeConfig.Name, fsPath)
	} else {
		log.Printf("Warning: No path specified for static route '%s'. Skipping registration.", routeConfig.Name)
		return nil
	}

	// Validate the path exists
	fileInfo, statErr := os.Stat(fsPath)
	if statErr != nil {
		log.Printf("Warning: Invalid path '%s' for static route '%s': %v. Skipping registration.", fsPath, routeConfig.Name, statErr)
		return nil
	}

	isDir = fileInfo.IsDir()
	log.Printf("Static Route [%s]: Path '%s' is directory: %t", routeConfig.Name, fsPath, isDir)

	// Check if removeFromPath is used with static routes (not applicable)
	if routeConfig.RemoveFromPath != "" {
		log.Printf("Warning: Static Route [%s]: 'removeFromPath' field (%s) is not applicable to static routes and will be ignored. This field is only used for proxy routes.",
			routeConfig.Name, routeConfig.RemoveFromPath)
	}

	if isDir {
		// Directory serving
		fs := http.Dir(fsPath) // Serve from the resolved directory path
		fileServer := http.FileServer(fs)

		// For static routes, determine if we should strip the route prefix
		routePrefix := routeConfig.From
		if strings.HasSuffix(routePrefix, "/*") {
			routePrefix = strings.TrimSuffix(routePrefix, "*") // Keep trailing slash: /dashboard/
		}

		// Check if the target directory contains subdirectories that match the route prefix
		// This helps decide whether to preserve the full URL path or strip the prefix
		shouldPreserveFullPath := false
		if routePrefix != "/" && len(routePrefix) > 1 {
			// Extract the first path component from the route prefix
			trimmedPrefix := strings.Trim(routePrefix, "/")
			firstComponent := strings.Split(trimmedPrefix, "/")[0]

			// Check if a subdirectory with this name exists in the target folder
			potentialSubdir := filepath.Join(fsPath, firstComponent)
			if stat, err := os.Stat(potentialSubdir); err == nil && stat.IsDir() {
				shouldPreserveFullPath = true
				log.Printf("Static Route [%s]: Found matching subdirectory '%s', preserving full URL path", routeConfig.Name, firstComponent)
			}
		}

		log.Printf("Static Route [%s]: Setting up directory serving - fsPath: %s, From: %s, routePrefix: %s, preserveFullPath: %t",
			routeConfig.Name, fsPath, routeConfig.From, routePrefix, shouldPreserveFullPath)

		// Choose handler based on whether to preserve full path
		var finalHandler http.Handler
		if routePrefix == "/" || shouldPreserveFullPath {
			finalHandler = fileServer
			log.Printf("Static Route [%s]: Using direct file server handler (preserving full URL path)", routeConfig.Name)
		} else {
			finalHandler = http.StripPrefix(routePrefix, fileServer)
			log.Printf("Static Route [%s]: Using StripPrefix handler with prefix: %s", routeConfig.Name, routePrefix)
		}

		// Wrap with SPA handler if needed
		if routeConfig.IsSPA {
			finalHandler = g.createSPAHandler(finalHandler, fsPath, routeConfig)
			log.Printf("Static Route [%s]: Wrapped with SPA handler", routeConfig.Name)
		}

		return func(w http.ResponseWriter, r *http.Request) {
			finalHandler.ServeHTTP(w, r)
		}

	} else {
		// Single file serving
		filePath := fsPath // Already cleaned in loadConfig
		log.Printf("Static Route [%s]: Setting up single file serving - filePath: %s", routeConfig.Name, filePath)

		// For single file routes, we need to handle both with and without trailing slash
		// Register the handler for both patterns to avoid redirects
		return func(w http.ResponseWriter, r *http.Request) {

			// Check existence/type at request time
			fileInfo, err := os.Stat(filePath)
			if err != nil {
				if os.IsNotExist(err) {
					log.Printf("Static Route [%s]: File not found: %s - returning 404", routeConfig.Name, filePath)
					http.NotFound(w, r)
				} else {
					log.Printf("Static Route [%s]: Error accessing static file (%s): %v - returning 500", routeConfig.Name, filePath, err)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
				return
			}
			if fileInfo.IsDir() {
				log.Printf("Static Route [%s]: Configuration Error - path points to directory %s but route is not configured for directory serving (/*)", routeConfig.Name, filePath)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			http.ServeFile(w, r, filePath)
		}
	}
}
