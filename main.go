package main

// gl:readme.md
import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// gl:readme.md#L20
type Config struct {
	Groups map[string]GroupConfig `yaml:"groups"`
}

type GroupConfig struct {
	LogFile        string         `yaml:"log_file"`
	TargetBase     string         `yaml:"target_base"`
	Domains        []DomainConfig `yaml:"domains"`
	AllowedSchemes []string       `yaml:"allowed_schemes"`
	PreservePath   bool           `yaml:"preserve_path"` // Default false - strips URL path
}

type DomainConfig struct {
	Name           string   `yaml:"name"`
	AllowedSchemes []string `yaml:"allowed_schemes"`
}

type LogEntry struct {
	Timestamp string `json:"ts"`
	Group     string `json:"group"`
	Scheme    string `json:"scheme"`
	Host      string `json:"host"`
	Path      string `json:"path"`
	IP        string `json:"ip"`
	Referer   string `json:"referer"`
	UA        string `json:"ua"`
	Target    string `json:"target"`
}

type DomainInfo struct {
	GroupName      string
	AllowedSchemes []string
}

var (
	config       Config
	domainToInfo map[string]DomainInfo
	logFiles     map[string]*os.File
	logMutex     sync.Mutex
)

func main() {
	configFile := flag.String("config", "config.yaml", "Path to config file")
	logDir := flag.String("log-dir", "/var/log/redirects", "Directory for log files")
	port := flag.Int("port", 8080, "Port to listen on")
	flag.Parse()

	// Load config
	data, err := os.ReadFile(*configFile)
	if err != nil {
		log.Fatalf("Error reading config: %v", err)
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		log.Fatalf("Error parsing config: %v", err)
	}

	// Initialize maps
	domainToInfo = make(map[string]DomainInfo)
	logFiles = make(map[string]*os.File)

	// Ensure log directory exists
	if err := os.MkdirAll(*logDir, 0755); err != nil {
		log.Fatalf("Error creating log directory: %v", err)
	}

	for groupName, group := range config.Groups {
		for _, domainCfg := range group.Domains {
			domainToInfo[strings.ToLower(domainCfg.Name)] = DomainInfo{
				GroupName:      groupName,
				AllowedSchemes: domainCfg.AllowedSchemes,
			}
		}

		// Open log file for each group
		f, err := os.OpenFile(filepath.Join(*logDir, group.LogFile), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Fatalf("Error opening log file for group %s: %v", groupName, err)
		}
		logFiles[groupName] = f
	}

	http.HandleFunc("/", handleRedirect)
	http.HandleFunc("/health", handleHealth)

	log.Printf("Starting redirect service on port %d...", *port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", *port), nil); err != nil {
		log.Fatal(err)
	}
}

// gl:docs/dev/go-redir-svc.md#L18
func handleRedirect(w http.ResponseWriter, r *http.Request) {
	host := strings.ToLower(r.Host)
	// Remove port if present
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}

	info, ok := domainToInfo[host]
	if !ok {
		http.Error(w, "Domain not configured", http.StatusNotFound)
		return
	}

	groupName := info.GroupName
	group := config.Groups[groupName]

	// Determine scheme (check X-Forwarded-Proto from Gateway)
	scheme := r.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	scheme = strings.ToLower(scheme)

	// Check if scheme is allowed (Domain override takes precedence)
	allowedSchemes := info.AllowedSchemes
	if len(allowedSchemes) == 0 {
		allowedSchemes = group.AllowedSchemes
	}

	allowed := false
	if len(allowedSchemes) == 0 {
		allowed = true // Default to both if not specified anywhere
	} else {
		for _, s := range allowedSchemes {
			if strings.ToLower(s) == scheme {
				allowed = true
				break
			}
		}
	}

	if !allowed {
		http.Error(w, fmt.Sprintf("Scheme %s not allowed for domain %s (Misdirected Request)", scheme, host), http.StatusMisdirectedRequest)
		return
	}

	// Build target URL
	target := group.TargetBase
	if group.PreservePath {
		target += r.URL.Path
	}
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}

	// Log the request
	entry := LogEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		Group:     groupName,
		Scheme:    scheme,
		Host:      host,
		Path:      r.URL.Path,
		IP:        getIP(r),
		Referer:   r.Header.Get("Referer"),
		UA:        r.Header.Get("User-Agent"),
		Target:    target,
	}

	logJSON(groupName, entry)

	http.Redirect(w, r, target, http.StatusMovedPermanently)
}

func getIP(r *http.Request) string {
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return strings.Split(ip, ",")[0]
	}
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	return r.RemoteAddr
}

func logJSON(groupName string, entry LogEntry) {
	logMutex.Lock()
	defer logMutex.Unlock()

	f, ok := logFiles[groupName]
	if !ok {
		return
	}

	data, err := json.Marshal(entry)
	if err != nil {
		log.Printf("Error marshaling log entry: %v", err)
		return
	}

	if _, err := f.Write(append(data, '\n')); err != nil {
		log.Printf("Error writing to log file: %v", err)
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok"}`)
}
