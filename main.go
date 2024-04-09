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
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const replicateToken = "Bearer replicate.com"

type Input struct {
	TopK             int     `json:"top_k"`
	TopP             float64 `json:"top_p"`
	Prompt           string  `json:"prompt"`
	Temperature      float64 `json:"temperature"`
	MaxNewTokens     int     `json:"max_new_tokens"`
	PromptTemplate   string  `json:"prompt_template"`
	PresencePenalty  float64 `json:"presence_penalty"`
	FrequencyPenalty float64 `json:"frequency_penalty"`
}

type AIRequest struct {
	Input Input `json:"input"`
}

type AIErrorResponse struct {
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Status int    `json:"status"`
}

type AIResponseUri struct {
	URLs struct {
		Cancel string `json:"cancel"`
		Get    string `json:"get"`
	} `json:"urls"`
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

func getAISmsContent(prompt string, logger *log.Logger) (*AIResponseUri, error) {
	// Call external AI service
	aiResponse, err := callAIService(prompt, logger)
	if err != nil {
		return nil, err
	}

	return aiResponse, nil
}

func callAIService(prompt string, logger *log.Logger) (*AIResponseUri, error) {
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
	requestBody := AIRequest{
		Input: Input{
			TopK:             50,
			TopP:             0.9,
			Prompt:           prompt,
			Temperature:      0.6,
			MaxNewTokens:     1024,
			PromptTemplate:   "<s>[INST] {prompt} [/INST] ",
			PresencePenalty:  0,
			FrequencyPenalty: 0,
		},
	}
	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		logger.Printf("Error marshaling request body: %v", err)
		return nil, err
	}
	logger.Printf("Calling AI service with request body: %s", string(jsonBody))

	req, err := http.NewRequest("POST", "https://api.replicate.com/v1/models/mistralai/mixtral-8x7b-instruct-v0.1/predictions", bytes.NewBuffer(jsonBody))
	if err != nil {
		logger.Printf("Error creating request: %v", err)
		return nil, err
	}
	req.Header.Add("Authorization", replicateToken)
	req.Header.Add("Content-Type", "application/json")

	resp, err := client.Do(req)
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
	logger.Printf("AI service response: %s", string(body))

	if resp.StatusCode != 201 {
		logger.Printf("Error calling AI service: status code %d", resp.StatusCode)
		var aiErrorResponse AIErrorResponse
		err = json.Unmarshal(body, &aiErrorResponse)
		if err != nil {
			logger.Printf("Error unmarshaling AI service ERROR response: %v", err)
			return nil, nil
		}

	}

	var AIResponseUri AIResponseUri
	err = json.Unmarshal(body, &AIResponseUri)
	if err != nil {
		logger.Printf("Error unmarshaling AI service response URI: %v", err)
		return nil, err
	}

	logger.Printf("result AI URI: %s", AIResponseUri.URLs.Get)

	// Call AI service again
	req, err = http.NewRequest("GET", AIResponseUri.URLs.Get, nil)
	if err != nil {
		logger.Printf("result Error creating req AI answer: %v", err)
		return nil, err
	}
	req.Header.Add("Authorization", replicateToken)
	req.Header.Add("Content-Type", "application/json")

	start := time.Now()
	resp, err = client.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		logger.Printf("result Error calling AI service: %v (elapsed %s)", err, elapsed)
		return nil, err
	}
	defer resp.Body.Close()

	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		logger.Printf("Error reading AI service response: %v (elapsed %s)", err, elapsed)
		return nil, err
	}
	logger.Printf("result AI service response (elapsed %s): %s", elapsed, string(body))

	return &AIResponseUri, nil
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
