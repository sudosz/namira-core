package main

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

	"github.com/gin-gonic/gin"
)

const (
	DefaultTimeout = 10 * time.Second
	MaxConcurrent  = 50
	TestURL        = "http://www.google.com"
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

func (rps *RayPingService) parseVmessLink(link string) (*LinkTestResult, error) {
	// Remove vmess:// prefix and decode base64
	encoded := strings.TrimPrefix(link, "vmess://")
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode vmess link: %v", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(decoded, &config); err != nil {
		return nil, fmt.Errorf("failed to parse vmess config: %v", err)
	}

	server, _ := config["add"].(string)
	portStr, _ := config["port"].(string)
	port, _ := strconv.Atoi(portStr)

	return &LinkTestResult{
		Link:     link,
		Protocol: "vmess",
		Server:   server,
		Port:     port,
	}, nil
}

func (rps *RayPingService) parseVlessLink(link string) (*LinkTestResult, error) {
	u, err := url.Parse(link)
	if err != nil {
		return nil, fmt.Errorf("failed to parse vless URL: %v", err)
	}

	port, _ := strconv.Atoi(u.Port())
	if port == 0 {
		port = 443 // default VLESS port
	}

	return &LinkTestResult{
		Link:     link,
		Protocol: "vless",
		Server:   u.Hostname(),
		Port:     port,
	}, nil
}

func (rps *RayPingService) parseShadowsocksLink(link string) (*LinkTestResult, error) {
	// Remove ss:// prefix
	encoded := strings.TrimPrefix(link, "ss://")

	// Handle both old and new SS URL formats
	var server string
	var port int

	if strings.Contains(encoded, "@") {
		// New format: ss://base64(method:password)@server:port
		parts := strings.Split(encoded, "@")
		if len(parts) == 2 {
			serverPort := parts[1]
			serverPortParts := strings.Split(serverPort, ":")
			if len(serverPortParts) >= 2 {
				server = serverPortParts[0]
				port, _ = strconv.Atoi(serverPortParts[1])
			}
		}
	} else {
		// Old format or other variations - try to extract server info
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("failed to decode shadowsocks link: %v", err)
		}

		// Try to parse as method:password@server:port
		decodedStr := string(decoded)
		if strings.Contains(decodedStr, "@") {
			parts := strings.Split(decodedStr, "@")
			if len(parts) == 2 {
				serverPort := parts[1]
				serverPortParts := strings.Split(serverPort, ":")
				if len(serverPortParts) >= 2 {
					server = serverPortParts[0]
					port, _ = strconv.Atoi(serverPortParts[1])
				}
			}
		}
	}

	return &LinkTestResult{
		Link:     link,
		Protocol: "ss",
		Server:   server,
		Port:     port,
	}, nil
}

func (rps *RayPingService) parseTrojanLink(link string) (*LinkTestResult, error) {
	u, err := url.Parse(link)
	if err != nil {
		return nil, fmt.Errorf("failed to parse trojan URL: %v", err)
	}

	port, _ := strconv.Atoi(u.Port())
	if port == 0 {
		port = 443 // default Trojan port
	}

	return &LinkTestResult{
		Link:     link,
		Protocol: "trojan",
		Server:   u.Hostname(),
		Port:     port,
	}, nil
}

func (rps *RayPingService) parseLink(link string) (*LinkTestResult, error) {
	link = strings.TrimSpace(link)

	switch {
	case strings.HasPrefix(link, "vmess://"):
		return rps.parseVmessLink(link)
	case strings.HasPrefix(link, "vless://"):
		return rps.parseVlessLink(link)
	case strings.HasPrefix(link, "ss://"):
		return rps.parseShadowsocksLink(link)
	case strings.HasPrefix(link, "trojan://"):
		return rps.parseTrojanLink(link)
	default:
		return nil, fmt.Errorf("unsupported protocol")
	}
}

func (rps *RayPingService) testConnection(result *LinkTestResult) {
	start := time.Now()
	result.TestedAt = start.Format(time.RFC3339)

	if result.Server == "" || result.Port == 0 {
		result.Failed = true
		result.Error = "invalid server or port"
		return
	}

	// Test TCP connection to the server
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", result.Server, result.Port), rps.timeout)
	if err != nil {
		result.Failed = true
		result.Error = fmt.Sprintf("connection failed: %v", err)
		return
	}
	defer conn.Close()

	// Calculate ping time
	pingTime := time.Since(start)
	result.PingMs = pingTime.Nanoseconds() / 1000000 // Convert to milliseconds
	result.Failed = false
}

func (rps *RayPingService) TestLinks(links []string) *TestResponse {
	totalLinks := len(links)
	results := make([]LinkTestResult, 0, totalLinks)
	resultChan := make(chan LinkTestResult, totalLinks)

	// Create a semaphore to limit concurrent connections
	semaphore := make(chan struct{}, rps.maxConcurrent)
	var wg sync.WaitGroup

	// Process links concurrently
	for _, link := range links {
		if link == "" {
			continue
		}

		wg.Add(1)
		go func(link string) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Parse the link
			result, err := rps.parseLink(link)
			if err != nil {
				result = &LinkTestResult{
					Link:     link,
					Protocol: "unknown",
					Failed:   true,
					Error:    fmt.Sprintf("parse error: %v", err),
					TestedAt: time.Now().Format(time.RFC3339),
				}
			} else {
				// Test the connection
				rps.testConnection(result)
			}

			resultChan <- *result
		}(link)
	}

	// Close result channel when all goroutines are done
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results
	for result := range resultChan {
		results = append(results, result)
	}

	// Sort results by ping time (working links first, then by ping time)
	sort.Slice(results, func(i, j int) bool {
		if results[i].Failed != results[j].Failed {
			return !results[i].Failed // Working links first
		}
		if results[i].Failed && results[j].Failed {
			return false // Keep failed links in original order
		}
		return results[i].PingMs < results[j].PingMs
	})

	// Count working links
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

func (rps *RayPingService) handleTest(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded"})
		return
	}
	defer file.Close()

	log.Printf("Processing file: %s", header.Filename)

	// Read links from file
	var links []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			links = append(links, line)
		}
	}

	if err := scanner.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read file"})
		return
	}

	log.Printf("Found %d links to test", len(links))

	// Test the links
	response := rps.TestLinks(links)

	log.Printf("Testing completed: %d total, %d working, %d failed",
		response.TotalTested, response.WorkingLinks, response.FailedLinks)

	c.JSON(http.StatusOK, response)
}

func (rps *RayPingService) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "healthy",
		"service": "rayping",
		"version": "1.0.0",
		"time":    time.Now().Format(time.RFC3339),
	})
}

func main() {
	// Parse command line arguments or use defaults
	timeout := DefaultTimeout
	maxConcurrent := MaxConcurrent
	port := "8080"

	// Create service
	service := NewRayPingService(timeout, maxConcurrent)

	// Setup Gin router
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()

	// Add CORS middleware
	router.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// Routes
	router.GET("/health", service.handleHealth)
	router.POST("/test", service.handleTest)
	router.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"service": "RayPing VPN Link Tester",
			"version": "1.0.0",
			"endpoints": []string{
				"GET /health - Health check",
				"POST /test - Test VPN links (upload file)",
			},
		})
	})

	log.Printf("RayPing service starting on port %s", port)
	log.Printf("Timeout: %v, Max concurrent: %d", timeout, maxConcurrent)

	if err := router.Run(":" + port); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}
