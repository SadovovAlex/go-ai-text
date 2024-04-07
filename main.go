package main

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type AIResponse struct {
	Variants []string `json:"variants"`
}

var (
	requestCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "ai_sms_requests_total",
		Help: "Total number of AI SMS requests",
	})
)

func main() {
	// Set up logging
	logFile, err := os.OpenFile("ai_sms_service.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	defer logFile.Close()
	logger := log.New(io.MultiWriter(logFile, os.Stdout), "", log.LstdFlags|log.Lmicroseconds)

	// Set up Prometheus metrics
	http.Handle("/metrics", promhttp.Handler())
	go func() {
		logger.Println("Starting Prometheus metrics server on :8082")
		err := http.ListenAndServe(":8082", nil)
		if err != nil {
			logger.Fatalf("Failed to start Prometheus metrics server: %v", err)
		}
	}()

	// Set up web server
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "index.html")
	})
	http.HandleFunc("/getAiSmsContent", func(w http.ResponseWriter, r *http.Request) {
		requestCounter.Inc()
		prompt := r.FormValue("prompt")
		logger.Printf("Received request for AI SMS content with prompt: %s", prompt)

		aiResponse, err := getAISmsContent(prompt, logger)
		if err != nil {
			logger.Printf("Error getting AI SMS content: %v", err)
			http.Error(w, "Error getting AI SMS content", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		err = json.NewEncoder(w).Encode(aiResponse)
		if err != nil {
			logger.Printf("Error encoding AI SMS response: %v", err)
			http.Error(w, "Error encoding AI SMS response", http.StatusInternalServerError)
			return
		}
	})

	logger.Println("Starting web server on :8080")
	err = http.ListenAndServe(":8080", nil)
	if err != nil {
		logger.Fatalf("Failed to start web server: %v", err)
	}
}

func getAISmsContent(prompt string, logger *log.Logger) (*AIResponse, error) {
	// Call external AI service
	aiResponse, err := callAIService(prompt, logger)
	if err != nil {
		return nil, err
	}

	return aiResponse, nil
}

func callAIService(prompt string, logger *log.Logger) (*AIResponse, error) {
	// Check if corporate proxy is set
	proxyURL, err := getProxyURL()
	if err != nil {
		logger.Printf("Error getting proxy URL: %v", err)
		return nil, err
	}

	// Create HTTP client with proxy
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
	}

	// Call AI service
	requestBody := map[string]string{"prompt": prompt}
	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		logger.Printf("Error marshaling request body: %v", err)
		return nil, err
	}

	resp, err := client.Post("https://api.example.com/generate-text", "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		logger.Printf("Error calling AI service: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logger.Printf("Error reading AI service response: %v", err)
		return nil, err
	}

	var aiResponse AIResponse
	err = json.Unmarshal(body, &aiResponse)
	if err != nil {
		logger.Printf("Error unmarshaling AI service response: %v", err)
		return nil, err
	}

	return &aiResponse, nil
}

func getProxyURL() (*url.URL, error) {
	proxyHost := os.Getenv("HTTP_PROXY")
	if proxyHost == "" {
		proxyHost = os.Getenv("HTTPS_PROXY")
	}
	if proxyHost == "" {
		return nil, nil
	}

	return url.Parse(proxyHost)
}
