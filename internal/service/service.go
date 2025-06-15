package service

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
)

const (
	DefaultTimeout = 10 * time.Second
	MaxConcurrent  = 50
	DefaultPort    = 443
)

type LinkTestResult struct {
	Link     string `json:"link"`
	Protocol string `json:"protocol"`
	PingMs   int64  `json:"ping_ms"`
	Failed   bool   `json:"failed"`
	Error    string `json:"error,omitempty"`
	Server   string `json:"server,omitempty"`
	Port     int    `json:"port,omitempty"`
	TestedAt string `json:"tested_at"`
}

type TestResponse struct {
	TotalTested  int              `json:"total_tested"`
	WorkingLinks int              `json:"working_links"`
	FailedLinks  int              `json:"failed_links"`
	Results      []LinkTestResult `json:"results"`
	ProcessedAt  string           `json:"processed_at"`
}

type RayPingService struct {
	timeout       time.Duration
	maxConcurrent int
}

func NewRayPingService(timeout time.Duration, maxConcurrent int) *RayPingService {
	return &RayPingService{
		timeout:       timeout,
		maxConcurrent: maxConcurrent,
	}
}

func parseServerAndPort(u *url.URL) (string, int) {
	port, _ := strconv.Atoi(u.Port())
	if port == 0 {
		port = DefaultPort
	}
	return u.Hostname(), port
}

func (rps *RayPingService) parseLink(link string) (*LinkTestResult, error) {
	link = strings.TrimSpace(link)
	result := &LinkTestResult{Link: link}

	switch {
	case strings.HasPrefix(link, "vmess://"):
		encoded := strings.TrimPrefix(link, "vmess://")
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("invalid vmess link: %v", err)
		}

		var config map[string]any
		if err := json.Unmarshal(decoded, &config); err != nil {
			return nil, fmt.Errorf("invalid vmess config: %v", err)
		}

		result.Protocol = "vmess"
		result.Server, _ = config["add"].(string)
		portStr, _ := config["port"].(string)
		result.Port, _ = strconv.Atoi(portStr)

	case strings.HasPrefix(link, "vless://"), strings.HasPrefix(link, "trojan://"):
		u, err := url.Parse(link)
		if err != nil {
			return nil, fmt.Errorf("invalid URL: %v", err)
		}
		result.Protocol = strings.TrimSuffix(u.Scheme, "://")
		result.Server, result.Port = parseServerAndPort(u)

	case strings.HasPrefix(link, "ss://"):
		encoded := strings.TrimPrefix(link, "ss://")
		result.Protocol = "ss"

		if idx := strings.Index(encoded, "@"); idx != -1 {
			serverPort := encoded[idx+1:]
			parts := strings.Split(serverPort, ":")
			if len(parts) >= 2 {
				result.Server = parts[0]
				result.Port, _ = strconv.Atoi(parts[1])
			}
		} else if decoded, err := base64.StdEncoding.DecodeString(encoded); err == nil {
			if idx := strings.Index(string(decoded), "@"); idx != -1 {
				serverPort := strings.Split(string(decoded)[idx+1:], ":")
				if len(serverPort) >= 2 {
					result.Server = serverPort[0]
					result.Port, _ = strconv.Atoi(serverPort[1])
				}
			}
		}

	default:
		return nil, fmt.Errorf("unsupported protocol")
	}

	return result, nil
}

func (rps *RayPingService) testConnection(result *LinkTestResult) {
	start := time.Now()
	result.TestedAt = start.Format(time.RFC3339)

	if result.Server == "" || result.Port == 0 {
		result.Failed = true
		result.Error = "invalid server or port"
		return
	}

	conn, err := net.DialTimeout("tcp", net.JoinHostPort(result.Server, strconv.Itoa(result.Port)), rps.timeout)
	if err != nil {
		result.Failed = true
		result.Error = fmt.Sprintf("connection failed: %v", err)
		return
	}
	defer conn.Close()

	result.PingMs = time.Since(start).Milliseconds()
}

func (rps *RayPingService) TestLinks(links []string) *TestResponse {
	results := make([]LinkTestResult, 0, len(links))
	resultChan := make(chan LinkTestResult, len(links))
	sem := make(chan struct{}, rps.maxConcurrent)
	var wg sync.WaitGroup

	for _, link := range links {
		if link == "" {
			continue
		}

		wg.Add(1)
		go func(link string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			result, err := rps.parseLink(link)
			if err != nil {
				resultChan <- LinkTestResult{
					Link:     link,
					Protocol: "unknown",
					Failed:   true,
					Error:    err.Error(),
					TestedAt: time.Now().Format(time.RFC3339),
				}
				return
			}

			rps.testConnection(result)
			resultChan <- *result
		}(link)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	for result := range resultChan {
		results = append(results, result)
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Failed != results[j].Failed {
			return !results[i].Failed
		}
		if results[i].Failed && results[j].Failed {
			return false // Keep failed links in original order
		}
		return results[i].PingMs < results[j].PingMs
	})

	workingLinks := 0
	for _, result := range results {
		if !result.Failed {
			workingLinks++
		}
	}

	return &TestResponse{
		TotalTested:  len(results),
		WorkingLinks: workingLinks,
		FailedLinks:  len(results) - workingLinks,
		Results:      results,
		ProcessedAt:  time.Now().Format(time.RFC3339),
	}
}

func (rps *RayPingService) handleTest(w http.ResponseWriter, r *http.Request) {
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "No file uploaded", http.StatusBadRequest)
		return
	}
	defer file.Close()

	var links []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); line != "" {
			links = append(links, line)
		}
	}

	if err := scanner.Err(); err != nil {
		http.Error(w, "Failed to read file", http.StatusInternalServerError)
		return
	}

	log.Printf("Testing %d links", len(links))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rps.TestLinks(links))
}

func (rps *RayPingService) StartServer(port string) error {
	router := mux.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	})

	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"service":   "RayPing VPN Link Tester",
			"version":   "1.0.0",
			"endpoints": []string{"GET /health", "POST /test"},
		})
	}).Methods("GET")

	router.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "healthy",
			"service": "rayping",
			"version": "1.0.0",
			"time":    time.Now().Format(time.RFC3339),
		})
	}).Methods("GET")

	router.HandleFunc("/test", rps.handleTest).Methods("POST")

	return (&http.Server{
		Addr:         ":" + port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}).ListenAndServe()
}
